package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

// PlayClientboundLoginPacket is Login (play) (0x29).
type PlayClientboundLoginPacket struct {
	EntityID int32
	TailData []byte
}

func (p *PlayClientboundLoginPacket) PacketID() int32 { return consts.PlayClientboundLogin }
func (p *PlayClientboundLoginPacket) Marshal(w *Writer) error {
	if err := w.WriteInt(p.EntityID); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundLoginPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadInt()
	if err != nil {
		return err
	}
	p.EntityID = id
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.TailData = tail
	return nil
}

// PlayClientboundSynchronizePlayerPositionPacket is Synchronize Player Position (0x3E).
type PlayClientboundSynchronizePlayerPositionPacket struct {
	X          float64
	Y          float64
	Z          float64
	Yaw        float32
	Pitch      float32
	Flags      byte
	TeleportID int32
	TailData   []byte
}

func (p *PlayClientboundSynchronizePlayerPositionPacket) PacketID() int32 {
	return consts.PlayClientboundSynchronizePlayerPosition
}
func (p *PlayClientboundSynchronizePlayerPositionPacket) Marshal(w *Writer) error {
	if err := w.WriteDouble(p.X); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Y); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Z); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Yaw); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Pitch); err != nil {
		return err
	}
	if err := w.WriteByte(p.Flags); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.TeleportID); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundSynchronizePlayerPositionPacket) Unmarshal(r *Reader) error {
	var err error
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Flags, err = r.ReadByte(); err != nil {
		return err
	}
	if p.TeleportID, err = r.ReadVarInt(); err != nil {
		return err
	}
	p.TailData, err = r.ReadRemainingBytes()
	return err
}

// PlayClientboundSystemChatMessagePacket is System Chat Message (0x69).
type PlayClientboundSystemChatMessagePacket struct {
	Message string
	Overlay bool
}

func (p *PlayClientboundSystemChatMessagePacket) PacketID() int32 {
	return consts.PlayClientboundSystemChatMessage
}
func (p *PlayClientboundSystemChatMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Message); err != nil {
		return err
	}
	return w.WriteBoolean(p.Overlay)
}
func (p *PlayClientboundSystemChatMessagePacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		p.Message = ""
		p.Overlay = false
		return nil
	}
	p.Overlay = data[len(data)-1] != 0
	content := data[:len(data)-1]
	if msg, ok := tryReadLegacyStringComponent(content); ok {
		p.Message = msg
		return nil
	}
	if msg := decodeNBTComponentText(content); msg != "" {
		p.Message = msg
		return nil
	}
	p.Message = bestEffortReadableString(content)
	return nil
}

// DisplayText returns a readable chat string for simple JSON text components.
func (p *PlayClientboundSystemChatMessagePacket) DisplayText() string {
	return componentPlainText(p.Message)
}

// PlayClientboundPlayerChatMessagePacket is Player Chat Message (0x37).
type PlayClientboundPlayerChatMessagePacket struct {
	RawData      []byte
	Sender       [16]byte
	Index        int32
	PlainMessage string
	SenderName   string
}

func (p *PlayClientboundPlayerChatMessagePacket) PacketID() int32 {
	return consts.PlayClientboundPlayerChatMessage
}
func (p *PlayClientboundPlayerChatMessagePacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.RawData)
	return err
}
func (p *PlayClientboundPlayerChatMessagePacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	p.parsePlayerChatData(data)
	return nil
}

func (p *PlayClientboundPlayerChatMessagePacket) parsePlayerChatData(data []byte) {
	r := NewReader(bytes.NewReader(data))
	sender, err := r.ReadUUID()
	if err != nil {
		p.PlainMessage = bestEffortReadableString(data)
		return
	}
	index, err := r.ReadVarInt()
	if err != nil {
		p.PlainMessage = bestEffortReadableString(data)
		return
	}
	hasSignature, err := r.ReadBoolean()
	if err != nil {
		p.PlainMessage = bestEffortReadableString(data)
		return
	}
	if hasSignature {
		sig := make([]byte, 256)
		if _, err := io.ReadFull(r.r, sig); err != nil {
			p.PlainMessage = bestEffortReadableString(data)
			return
		}
	}
	plain, err := r.ReadString()
	if err != nil {
		p.PlainMessage = bestEffortReadableString(data)
		return
	}
	if _, err := r.ReadLong(); err != nil {
		p.PlainMessage = plain
		return
	}
	if _, err := r.ReadLong(); err != nil {
		p.PlainMessage = plain
		return
	}
	previousCount, err := r.ReadVarInt()
	if err != nil {
		p.PlainMessage = plain
		return
	}
	if previousCount < 0 || previousCount > 20 {
		p.PlainMessage = plain
		return
	}
	for i := int32(0); i < previousCount; i++ {
		if _, err := r.ReadVarInt(); err != nil {
			p.PlainMessage = plain
			return
		}
		sig := make([]byte, 256)
		if _, err := io.ReadFull(r.r, sig); err != nil {
			p.PlainMessage = plain
			return
		}
	}
	hasUnsignedContent, err := r.ReadBoolean()
	if err != nil {
		p.PlainMessage = plain
		return
	}
	if hasUnsignedContent {
		if _, err := r.ReadString(); err != nil {
			p.PlainMessage = plain
			return
		}
	}
	filterType, err := r.ReadVarInt()
	if err != nil {
		p.PlainMessage = plain
		return
	}
	if filterType == 2 {
		if _, err := r.ReadBitSet(); err != nil {
			p.PlainMessage = plain
			return
		}
	}
	if _, err := r.ReadVarInt(); err != nil {
		p.PlainMessage = plain
		return
	}
	p.Sender = sender
	p.Index = index
	p.PlainMessage = plain
	if remaining, err := r.ReadRemainingBytes(); err == nil {
		if len(remaining) > 0 && (remaining[0] == 8 || remaining[0] == 10) {
			if senderName := decodeNBTComponentText(remaining); senderName != "" {
				p.SenderName = senderName
			}
			return
		}
		if senderName, ok := tryReadLegacyStringComponentPrefix(remaining); ok {
			p.SenderName = componentPlainText(senderName)
			return
		}
		if senderName := decodeNBTComponentText(remaining); senderName != "" {
			p.SenderName = senderName
		}
	}
}

// DisplayMessage returns a readable, non-corrupt player chat message.
func (p *PlayClientboundPlayerChatMessagePacket) DisplayMessage() string {
	message := cleanDisplayString(p.PlainMessage)
	if message == "" {
		message = cleanDisplayString(bestEffortReadableString(p.RawData))
	}
	sender := cleanDisplayString(p.SenderName)
	if sender == "" || len(sender) < 3 {
		return message
	}
	if strings.HasPrefix(message, sender+":") {
		return message
	}
	return sender + ": " + message
}

func componentPlainText(component string) string {
	component = strings.TrimSpace(component)
	if component == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(component), &decoded); err == nil {
		if text := flattenComponentText(decoded); text != "" {
			return cleanDisplayString(text)
		}
	}
	return cleanDisplayString(component)
}

func flattenComponentText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var b strings.Builder
		for _, part := range x {
			b.WriteString(flattenComponentText(part))
		}
		return b.String()
	case map[string]any:
		var b strings.Builder
		if text, ok := x["text"]; ok {
			b.WriteString(flattenComponentText(text))
		}
		if extra, ok := x["extra"]; ok {
			b.WriteString(flattenComponentText(extra))
		}
		if translate, ok := x["translate"].(string); ok && b.Len() == 0 {
			b.WriteString(translate)
			if withArr, ok := x["with"].([]any); ok {
				for _, param := range withArr {
					paramText := flattenComponentText(param)
					if paramText != "" {
						b.WriteString(" ")
						b.WriteString(paramText)
					}
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

func bestEffortReadableString(data []byte) string {
	best := ""
	for start := 0; start < len(data); {
		rn, size := utf8.DecodeRune(data[start:])
		if rn == utf8.RuneError && size == 1 {
			start++
			continue
		}
		if !isReadableChatRune(rn) {
			start += size
			continue
		}
		end := start + size
		for end < len(data) {
			rn, size = utf8.DecodeRune(data[end:])
			if rn == utf8.RuneError && size == 1 {
				break
			}
			if !isReadableChatRune(rn) {
				break
			}
			end += size
		}
		candidate := strings.TrimSpace(string(data[start:end]))
		if len(candidate) > len(best) {
			best = candidate
		}
		start = end + 1
	}
	return best
}

func isReadableChatRune(r rune) bool {
	return r == ' ' || r == ':' || r == '_' || r == '-' || r == '\'' || r == '"' ||
		r == '.' || r == ',' || r == '!' || r == '?' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func cleanDisplayString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\uFFFD' || r == 0 {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func tryReadLegacyStringComponent(data []byte) (string, bool) {
	rr := NewReader(bytes.NewReader(data))
	msg, err := rr.ReadString()
	if err != nil {
		return "", false
	}
	rest, err := rr.ReadRemainingBytes()
	if err != nil || len(rest) != 0 {
		return "", false
	}
	return msg, true
}

func tryReadLegacyStringComponentPrefix(data []byte) (string, bool) {
	rr := NewReader(bytes.NewReader(data))
	msg, err := rr.ReadString()
	if err != nil {
		return "", false
	}
	if cleanDisplayString(componentPlainText(msg)) == "" {
		return "", false
	}
	return msg, true
}

func decodeNBTComponentText(data []byte) string {
	text, _ := decodeNBTComponentTextWithConsumed(data)
	return text
}

func decodeNBTComponentTextWithConsumed(data []byte) (string, int) {
	if len(data) == 0 {
		return "", 0
	}
	if data[0] == 10 {
		for _, namedRoot := range []bool{false, true} {
			reader := &nbtComponentReader{data: data}
			_, _ = reader.readByte()
			if namedRoot {
				_ = reader.readString16()
			}
			var parts []string
			reader.collectCompoundText(&parts)
			if len(parts) > 0 {
				return cleanDisplayString(strings.Join(parts, " ")), reader.off
			}
		}
		return "", 0
	}
	if data[0] == 8 {
		reader := &nbtComponentReader{data: data}
		_, _ = reader.readByte()
		value := reader.readString16()
		return cleanDisplayString(value), reader.off
	}
	reader := &nbtComponentReader{data: data}
	var parts []string
	reader.collectCompoundText(&parts)
	if len(parts) == 0 {
		return "", 0
	}
	return cleanDisplayString(strings.Join(parts, " ")), reader.off
}

type nbtComponentReader struct {
	data []byte
	off  int
}

func (r *nbtComponentReader) remaining() int {
	return len(r.data) - r.off
}

func (r *nbtComponentReader) readByte() (byte, bool) {
	if r.remaining() < 1 {
		return 0, false
	}
	v := r.data[r.off]
	r.off++
	return v, true
}

func (r *nbtComponentReader) readInt16() (int, bool) {
	if r.remaining() < 2 {
		return 0, false
	}
	v := int(r.data[r.off])<<8 | int(r.data[r.off+1])
	r.off += 2
	return v, true
}

func (r *nbtComponentReader) readInt32() (int, bool) {
	if r.remaining() < 4 {
		return 0, false
	}
	v := int(r.data[r.off])<<24 | int(r.data[r.off+1])<<16 | int(r.data[r.off+2])<<8 | int(r.data[r.off+3])
	r.off += 4
	return v, true
}

func (r *nbtComponentReader) readString16() string {
	n, ok := r.readInt16()
	if !ok || n < 0 || r.remaining() < n {
		r.off = len(r.data)
		return ""
	}
	s := string(r.data[r.off : r.off+n])
	r.off += n
	return s
}

func (r *nbtComponentReader) skip(n int) {
	if n < 0 || r.remaining() < n {
		r.off = len(r.data)
		return
	}
	r.off += n
}

func (r *nbtComponentReader) collectCompoundText(parts *[]string) {
	for r.remaining() > 0 {
		tag, ok := r.readByte()
		if !ok || tag == 0 {
			return
		}
		name := r.readString16()
		r.collectPayloadText(tag, name, parts)
	}
}

func (r *nbtComponentReader) collectPayloadText(tag byte, name string, parts *[]string) {
	switch tag {
	case 1:
		r.skip(1)
	case 2:
		r.skip(2)
	case 3, 5:
		r.skip(4)
	case 4, 6:
		r.skip(8)
	case 7:
		n, ok := r.readInt32()
		if !ok {
			return
		}
		r.skip(n)
	case 8:
		value := r.readString16()
		if name == "text" || name == "translate" || name == "with" {
			*parts = append(*parts, value)
		}
	case 9:
		elemType, ok := r.readByte()
		if !ok {
			return
		}
		n, ok := r.readInt32()
		if !ok || n < 0 {
			return
		}
		for i := 0; i < n; i++ {
			r.collectPayloadText(elemType, name, parts)
		}
	case 10:
		r.collectCompoundText(parts)
	case 11:
		n, ok := r.readInt32()
		if !ok {
			return
		}
		r.skip(n * 4)
	case 12:
		n, ok := r.readInt32()
		if !ok {
			return
		}
		r.skip(n * 8)
	default:
		r.off = len(r.data)
	}
}

// PlayClientboundKeepAlivePacket is Clientbound Keep Alive (play) (0x24).
type PlayClientboundKeepAlivePacket struct {
	KeepAliveID int64
}

func (p *PlayClientboundKeepAlivePacket) PacketID() int32 {
	return consts.PlayClientboundClientboundKeepAlive
}
func (p *PlayClientboundKeepAlivePacket) Marshal(w *Writer) error { return w.WriteLong(p.KeepAliveID) }
func (p *PlayClientboundKeepAlivePacket) Unmarshal(r *Reader) error {
	id, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.KeepAliveID = id
	return nil
}

// PlayClientboundDisconnectPacket is Disconnect (play) (0x1B).
type PlayClientboundDisconnectPacket struct {
	Reason string
}

func (p *PlayClientboundDisconnectPacket) PacketID() int32         { return consts.PlayClientboundDisconnect }
func (p *PlayClientboundDisconnectPacket) Marshal(w *Writer) error { return w.WriteString(p.Reason) }
func (p *PlayClientboundDisconnectPacket) Unmarshal(r *Reader) error {
	reason, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Reason = reason
	return nil
}

// PlayClientboundChunkDataAndUpdateLightPacket is Chunk Data and Update Light (0x25).
type PlayClientboundChunkDataAndUpdateLightPacket struct {
	ChunkX   int32
	ChunkZ   int32
	TailData []byte
}

func (p *PlayClientboundChunkDataAndUpdateLightPacket) PacketID() int32 {
	return consts.PlayClientboundChunkDataAndUpdateLight
}
func (p *PlayClientboundChunkDataAndUpdateLightPacket) Marshal(w *Writer) error {
	if err := w.WriteInt(p.ChunkX); err != nil {
		return err
	}
	if err := w.WriteInt(p.ChunkZ); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundChunkDataAndUpdateLightPacket) Unmarshal(r *Reader) error {
	x, err := r.ReadInt()
	if err != nil {
		return err
	}
	z, err := r.ReadInt()
	if err != nil {
		return err
	}
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.ChunkX = x
	p.ChunkZ = z
	p.TailData = tail
	return nil
}

// PlayClientboundSetHealthPacket is Set Health (0x5B).
type PlayClientboundSetHealthPacket struct {
	Health         float32
	Food           int32
	Saturation     float32
	AdditionalData []byte
}

func (p *PlayClientboundSetHealthPacket) PacketID() int32 { return consts.PlayClientboundSetHealth }
func (p *PlayClientboundSetHealthPacket) Marshal(w *Writer) error {
	if err := w.WriteFloat(p.Health); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.Food); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Saturation); err != nil {
		return err
	}
	_, err := w.w.Write(p.AdditionalData)
	return err
}
func (p *PlayClientboundSetHealthPacket) Unmarshal(r *Reader) error {
	h, err := r.ReadFloat()
	if err != nil {
		return err
	}
	food, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	sat, err := r.ReadFloat()
	if err != nil {
		return err
	}
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.Health = h
	p.Food = food
	p.Saturation = sat
	p.AdditionalData = tail
	return nil
}

// PlayClientboundSetContainerContentPacket is Set Container Content (0x13).
type PlayClientboundSetContainerContentPacket struct {
	WindowID    int32
	StateID     int32
	SlotCount   int32
	Slots       []ItemStack
	CarriedItem ItemStack
	RawSlotData []byte
}

func (p *PlayClientboundSetContainerContentPacket) PacketID() int32 {
	return consts.PlayClientboundSetContainerContent
}
func (p *PlayClientboundSetContainerContentPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.WindowID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.StateID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.SlotCount); err != nil {
		return err
	}
	if len(p.Slots) > 0 {
		for _, slot := range p.Slots {
			if err := slot.Marshal(w); err != nil {
				return err
			}
		}
		return p.CarriedItem.Marshal(w)
	}
	_, err := w.w.Write(p.RawSlotData)
	return err
}
func (p *PlayClientboundSetContainerContentPacket) Unmarshal(r *Reader) error {
	windowID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	stateID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	slotCount, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	slots, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.WindowID = windowID
	p.StateID = stateID
	p.SlotCount = slotCount
	p.RawSlotData = slots
	if slotCount >= 0 && slotCount <= 512 {
		rr := NewReader(bytes.NewReader(slots))
		parsed := make([]ItemStack, 0, slotCount)
		ok := true
		for i := int32(0); i < slotCount; i++ {
			stack, err := ReadItemStack(rr)
			if err != nil {
				ok = false
				break
			}
			parsed = append(parsed, stack)
		}
		if ok {
			carried, err := ReadItemStack(rr)
			if err == nil {
				p.Slots = parsed
				p.CarriedItem = carried
			}
		}
	}
	return nil
}

// ItemStack is the minimal Protocol 765 slot format used by inventory smoke tests.
// It intentionally stores raw NBT only and writes TAG_End for simple no-NBT stacks.
type ItemStack struct {
	Present bool
	ItemID  int32
	Count   byte
	NBT     []byte
}

func (s ItemStack) Marshal(w *Writer) error {
	if err := w.WriteBoolean(s.Present); err != nil {
		return err
	}
	if !s.Present {
		return nil
	}
	if err := w.WriteVarInt(s.ItemID); err != nil {
		return err
	}
	if err := w.WriteByte(s.Count); err != nil {
		return err
	}
	nbt := s.NBT
	if len(nbt) == 0 {
		nbt = []byte{0x00}
	}
	_, err := w.w.Write(nbt)
	return err
}

type trackingReader struct {
	r   io.Reader
	buf bytes.Buffer
}

func (tr *trackingReader) Read(p []byte) (n int, err error) {
	n, err = tr.r.Read(p)
	if n > 0 {
		tr.buf.Write(p[:n])
	}
	return n, err
}

func skipNBTPayload(r *Reader, tagType byte) error {
	switch tagType {
	case 1: // Byte
		_, err := r.ReadByte()
		return err
	case 2: // Short
		_, err := r.ReadShort()
		return err
	case 3: // Int
		_, err := r.ReadInt()
		return err
	case 4: // Long
		_, err := r.ReadLong()
		return err
	case 5: // Float
		_, err := r.ReadFloat()
		return err
	case 6: // Double
		_, err := r.ReadDouble()
		return err
	case 7: // Byte Array
		lenVal, err := r.ReadInt()
		if err != nil {
			return err
		}
		if lenVal < 0 {
			return fmt.Errorf("negative NBT byte array length")
		}
		_, err = io.CopyN(io.Discard, r.r, int64(lenVal))
		return err
	case 8: // String
		lenVal, err := r.ReadShort()
		if err != nil {
			return err
		}
		if lenVal < 0 {
			return fmt.Errorf("negative NBT string length")
		}
		_, err = io.CopyN(io.Discard, r.r, int64(lenVal))
		return err
	case 9: // List
		elemType, err := r.ReadByte()
		if err != nil {
			return err
		}
		lenVal, err := r.ReadInt()
		if err != nil {
			return err
		}
		if lenVal < 0 {
			return fmt.Errorf("negative NBT list length")
		}
		for i := 0; i < int(lenVal); i++ {
			if err := skipNBTPayload(r, elemType); err != nil {
				return err
			}
		}
		return nil
	case 10: // Compound
		for {
			childType, err := r.ReadByte()
			if err != nil {
				return err
			}
			if childType == 0 { // TAG_End
				break
			}
			nameLen, err := r.ReadShort()
			if err != nil {
				return err
			}
			if nameLen < 0 {
				return fmt.Errorf("negative NBT compound name length")
			}
			_, err = io.CopyN(io.Discard, r.r, int64(nameLen))
			if err != nil {
				return err
			}
			if err := skipNBTPayload(r, childType); err != nil {
				return err
			}
		}
		return nil
	case 11: // Int Array
		lenVal, err := r.ReadInt()
		if err != nil {
			return err
		}
		if lenVal < 0 {
			return fmt.Errorf("negative NBT int array length")
		}
		_, err = io.CopyN(io.Discard, r.r, int64(lenVal)*4)
		return err
	case 12: // Long Array
		lenVal, err := r.ReadInt()
		if err != nil {
			return err
		}
		if lenVal < 0 {
			return fmt.Errorf("negative NBT long array length")
		}
		_, err = io.CopyN(io.Discard, r.r, int64(lenVal)*8)
		return err
	default:
		return fmt.Errorf("unknown NBT tag type: %d", tagType)
	}
}

func ReadItemStack(r *Reader) (ItemStack, error) {
	present, err := r.ReadBoolean()
	if err != nil {
		return ItemStack{}, err
	}
	if !present {
		return ItemStack{Present: false}, nil
	}
	itemID, err := r.ReadVarInt()
	if err != nil {
		return ItemStack{}, err
	}
	count, err := r.ReadByte()
	if err != nil {
		return ItemStack{}, err
	}
	tag, err := r.ReadByte()
	if err != nil {
		return ItemStack{}, err
	}
	if tag == 0 {
		return ItemStack{Present: true, ItemID: itemID, Count: count, NBT: []byte{0x00}}, nil
	}

	tr := &trackingReader{r: r.r}
	tr.buf.WriteByte(tag)
	trackingDec := NewReader(tr)
	if err := skipNBTPayload(trackingDec, tag); err != nil {
		return ItemStack{}, fmt.Errorf("failed to parse NBT: %w", err)
	}

	return ItemStack{
		Present: true,
		ItemID:  itemID,
		Count:   count,
		NBT:     tr.buf.Bytes(),
	}, nil
}

// PlayClientboundSetContainerSlotPacket is Set Container Slot (0x15).
type PlayClientboundSetContainerSlotPacket struct {
	WindowID int32
	StateID  int32
	Slot     int16
	Item     ItemStack
}

func (p *PlayClientboundSetContainerSlotPacket) PacketID() int32 {
	return consts.PlayClientboundSetContainerSlot
}
func (p *PlayClientboundSetContainerSlotPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.WindowID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.StateID); err != nil {
		return err
	}
	if err := w.WriteShort(p.Slot); err != nil {
		return err
	}
	return p.Item.Marshal(w)
}
func (p *PlayClientboundSetContainerSlotPacket) Unmarshal(r *Reader) error {
	var err error
	if p.WindowID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.StateID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.Slot, err = r.ReadShort(); err != nil {
		return err
	}
	p.Item, err = ReadItemStack(r)
	return err
}

type ClickContainerChangedSlot struct {
	Slot int16
	Item ItemStack
}

// PlayServerboundClickContainerPacket is Click Container (0x0D) for Protocol 765.
type PlayServerboundClickContainerPacket struct {
	WindowID     byte
	StateID      int32
	Slot         int16
	Button       byte
	Mode         int32
	ChangedSlots []ClickContainerChangedSlot
	CarriedItem  ItemStack
}

func (p *PlayServerboundClickContainerPacket) PacketID() int32 {
	return consts.PlayServerboundClickContainer
}

func (p *PlayServerboundClickContainerPacket) Marshal(w *Writer) error {
	if err := w.WriteByte(p.WindowID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.StateID); err != nil {
		return err
	}
	if err := w.WriteShort(p.Slot); err != nil {
		return err
	}
	if err := w.WriteByte(p.Button); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.Mode); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.ChangedSlots))); err != nil {
		return err
	}
	for _, changed := range p.ChangedSlots {
		if err := w.WriteShort(changed.Slot); err != nil {
			return err
		}
		if err := changed.Item.Marshal(w); err != nil {
			return err
		}
	}
	return p.CarriedItem.Marshal(w)
}

func (p *PlayServerboundClickContainerPacket) Unmarshal(r *Reader) error {
	windowID, err := r.ReadByte()
	if err != nil {
		return err
	}
	stateID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	slot, err := r.ReadShort()
	if err != nil {
		return err
	}
	button, err := r.ReadByte()
	if err != nil {
		return err
	}
	mode, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	changedCount, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	if changedCount < 0 || changedCount > 128 {
		return fmt.Errorf("invalid changed slots count: %d", changedCount)
	}
	changed := make([]ClickContainerChangedSlot, 0, changedCount)
	for i := int32(0); i < changedCount; i++ {
		changedSlot, err := r.ReadShort()
		if err != nil {
			return err
		}
		item, err := ReadItemStack(r)
		if err != nil {
			return err
		}
		changed = append(changed, ClickContainerChangedSlot{Slot: changedSlot, Item: item})
	}
	carried, err := ReadItemStack(r)
	if err != nil {
		return err
	}
	p.WindowID = windowID
	p.StateID = stateID
	p.Slot = slot
	p.Button = button
	p.Mode = mode
	p.ChangedSlots = changed
	p.CarriedItem = carried
	return nil
}

// PlayClientboundSetHeldItemPacket is Set Held Item (0x51).
type PlayClientboundSetHeldItemPacket struct {
	Slot byte
}

func (p *PlayClientboundSetHeldItemPacket) PacketID() int32 { return consts.PlayClientboundSetHeldItem }
func (p *PlayClientboundSetHeldItemPacket) Marshal(w *Writer) error {
	return w.WriteByte(p.Slot)
}
func (p *PlayClientboundSetHeldItemPacket) Unmarshal(r *Reader) error {
	slot, err := r.ReadByte()
	if err != nil {
		return err
	}
	p.Slot = slot
	return nil
}

// PlayServerboundConfirmTeleportationPacket is Confirm Teleportation (0x00).
type PlayServerboundConfirmTeleportationPacket struct {
	TeleportID int32
}

func (p *PlayServerboundConfirmTeleportationPacket) PacketID() int32 {
	return consts.PlayServerboundConfirmTeleportation
}
func (p *PlayServerboundConfirmTeleportationPacket) Marshal(w *Writer) error {
	return w.WriteVarInt(p.TeleportID)
}
func (p *PlayServerboundConfirmTeleportationPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.TeleportID = id
	return nil
}

// PlayClientboundSpawnEntityPacket is Spawn Entity (0x01).
type PlayClientboundSpawnEntityPacket struct {
	EntityID  int32
	UUID      [16]byte
	Type      int32
	X, Y, Z   float64
	Pitch     byte
	Yaw       byte
	HeadYaw   byte
	Data      int32
	VelocityX int16
	VelocityY int16
	VelocityZ int16
}

func (p *PlayClientboundSpawnEntityPacket) PacketID() int32 { return consts.PlayClientboundSpawnEntity }
func (p *PlayClientboundSpawnEntityPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(p.EntityID)
	_ = w.WriteUUID(p.UUID)
	_ = w.WriteVarInt(p.Type)
	_ = w.WriteDouble(p.X)
	_ = w.WriteDouble(p.Y)
	_ = w.WriteDouble(p.Z)
	_ = w.WriteByte(p.Pitch)
	_ = w.WriteByte(p.Yaw)
	_ = w.WriteByte(p.HeadYaw)
	_ = w.WriteVarInt(p.Data)
	_ = w.WriteShort(p.VelocityX)
	_ = w.WriteShort(p.VelocityY)
	_ = w.WriteShort(p.VelocityZ)
	return nil
}
func (p *PlayClientboundSpawnEntityPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.UUID, err = r.ReadUUID(); err != nil {
		return err
	}
	if p.Type, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadByte(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadByte(); err != nil {
		return err
	}
	if p.HeadYaw, err = r.ReadByte(); err != nil {
		return err
	}
	if p.Data, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.VelocityX, err = r.ReadShort(); err != nil {
		return err
	}
	if p.VelocityY, err = r.ReadShort(); err != nil {
		return err
	}
	if p.VelocityZ, err = r.ReadShort(); err != nil {
		return err
	}
	return nil
}

// PlayClientboundRemoveEntitiesPacket is Remove Entities (0x40).
type PlayClientboundRemoveEntitiesPacket struct {
	EntityIDs []int32
}

func (p *PlayClientboundRemoveEntitiesPacket) PacketID() int32 {
	return consts.PlayClientboundRemoveEntities
}
func (p *PlayClientboundRemoveEntitiesPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(int32(len(p.EntityIDs)))
	for _, id := range p.EntityIDs {
		_ = w.WriteVarInt(id)
	}
	return nil
}
func (p *PlayClientboundRemoveEntitiesPacket) Unmarshal(r *Reader) error {
	count, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.EntityIDs = make([]int32, count)
	for i := int32(0); i < count; i++ {
		id, err := r.ReadVarInt()
		if err != nil {
			return err
		}
		p.EntityIDs[i] = id
	}
	return nil
}

// PlayServerboundPlayerSessionPacket is Player Session (0x06).
type PlayServerboundPlayerSessionPacket struct {
	SessionID [16]byte
	ExpiresAt int64
	PublicKey []byte
	Signature []byte
}

func (p *PlayServerboundPlayerSessionPacket) PacketID() int32 {
	return consts.PlayServerboundPlayerSession
}
func (p *PlayServerboundPlayerSessionPacket) Marshal(w *Writer) error {
	if _, err := w.w.Write(p.SessionID[:]); err != nil {
		return err
	}
	if err := w.WriteLong(p.ExpiresAt); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.PublicKey))); err != nil {
		return err
	}
	if _, err := w.w.Write(p.PublicKey); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.Signature))); err != nil {
		return err
	}
	_, err := w.w.Write(p.Signature)
	return err
}
func (p *PlayServerboundPlayerSessionPacket) Unmarshal(r *Reader) error {
	if _, err := io.ReadFull(r.r, p.SessionID[:]); err != nil {
		return err
	}
	exp, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.ExpiresAt = exp
	pubLen, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.PublicKey = make([]byte, pubLen)
	if _, err := io.ReadFull(r.r, p.PublicKey); err != nil {
		return err
	}
	sigLen, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r.r, p.Signature); err != nil {
		return err
	}
	return nil
}

// PlayServerboundChatMessagePacket is Chat Message (0x05).
type PlayServerboundChatMessagePacket struct {
	Message      string
	Timestamp    int64
	Salt         int64
	MessageCount int32
}

func (p *PlayServerboundChatMessagePacket) PacketID() int32 { return consts.PlayServerboundChatMessage }
func (p *PlayServerboundChatMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Message); err != nil {
		return err
	}
	if err := w.WriteLong(p.Timestamp); err != nil {
		return err
	}
	if err := w.WriteLong(p.Salt); err != nil {
		return err
	}
	if err := w.WriteBoolean(false); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.MessageCount); err != nil {
		return err
	}
	// Acknowledged is Fixed BitSet(20) => 20 bits => 3 raw bytes, not length-prefixed.
	_, err := w.w.Write([]byte{0x00, 0x00, 0x00})
	return err
}
func (p *PlayServerboundChatMessagePacket) Unmarshal(r *Reader) error {
	msg, err := r.ReadString()
	if err != nil {
		return err
	}
	ts, err := r.ReadLong()
	if err != nil {
		return err
	}
	salt, err := r.ReadLong()
	if err != nil {
		return err
	}
	hasSig, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	if hasSig {
		sig := make([]byte, 256)
		if _, err := io.ReadFull(r.r, sig); err != nil {
			return err
		}
	}
	count, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	ack := make([]byte, 3)
	if _, err := io.ReadFull(r.r, ack); err != nil {
		return err
	}
	p.Message = msg
	p.Timestamp = ts
	p.Salt = salt
	p.MessageCount = count
	return nil
}

// PlayServerboundSetPlayerPositionAndRotationPacket is Set Player Position and Rotation (0x18).
type PlayServerboundSetPlayerPositionAndRotationPacket struct {
	X        float64
	Y        float64
	Z        float64
	Yaw      float32
	Pitch    float32
	OnGround bool
}

func (p *PlayServerboundSetPlayerPositionAndRotationPacket) PacketID() int32 {
	return consts.PlayServerboundSetPlayerPositionAndRotation
}
func (p *PlayServerboundSetPlayerPositionAndRotationPacket) Marshal(w *Writer) error {
	if err := w.WriteDouble(p.X); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Y); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Z); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Yaw); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Pitch); err != nil {
		return err
	}
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayServerboundSetPlayerPositionAndRotationPacket) Unmarshal(r *Reader) error {
	var err error
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadFloat(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

const (
	PlayerActionStartDigging       int32 = 0
	PlayerActionCancelDigging      int32 = 1
	PlayerActionFinishDigging      int32 = 2
	PlayerActionDropItemStack      int32 = 3
	PlayerActionDropItem           int32 = 4
	PlayerActionShootArrowOrFinish int32 = 5
	PlayerActionSwapItemInHand     int32 = 6

	BlockFaceBottom byte = 0
	BlockFaceTop    byte = 1
	BlockFaceNorth  byte = 2
	BlockFaceSouth  byte = 3
	BlockFaceWest   byte = 4
	BlockFaceEast   byte = 5

	MainHand int32 = 0
	OffHand  int32 = 1
)

type Direction byte

const (
	DirectionDown  Direction = 0
	DirectionUp    Direction = 1
	DirectionNorth Direction = 2
	DirectionSouth Direction = 3
	DirectionWest  Direction = 4
	DirectionEast  Direction = 5
)

// PlayServerboundPlayerActionPacket is Player Action (0x21).
type PlayServerboundPlayerActionPacket struct {
	RawData  []byte
	Status   int32
	Position BlockPos
	Face     byte
	Sequence int32
}

func (p *PlayServerboundPlayerActionPacket) PacketID() int32 {
	return consts.PlayServerboundPlayerAction
}
func (p *PlayServerboundPlayerActionPacket) Marshal(w *Writer) error {
	if p.RawData != nil {
		_, err := w.w.Write(p.RawData)
		return err
	}
	if err := w.WriteVarInt(p.Status); err != nil {
		return err
	}
	if err := writeBlockPos(w, p.Position); err != nil {
		return err
	}
	if err := w.WriteByte(p.Face); err != nil {
		return err
	}
	return w.WriteVarInt(p.Sequence)
}
func (p *PlayServerboundPlayerActionPacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	rr := NewReader(bytes.NewReader(data))
	status, err := rr.ReadVarInt()
	if err != nil {
		return nil
	}
	pos, err := readBlockPos(rr)
	if err != nil {
		return nil
	}
	face, err := rr.ReadByte()
	if err != nil {
		return nil
	}
	seq, err := rr.ReadVarInt()
	if err != nil {
		return nil
	}
	p.Status = status
	p.Position = pos
	p.Face = face
	p.Sequence = seq
	return nil
}

const (
	// PlayerCommandStartSprinting corresponds to the protocol action ID for starting sprint.
	PlayerCommandStartSprinting int32 = 3
	// PlayerCommandStopSprinting corresponds to the protocol action ID for stopping sprint.
	PlayerCommandStopSprinting int32 = 4
)

// PlayServerboundPlayerCommandPacket is Player Command (0x22).
type PlayServerboundPlayerCommandPacket struct {
	EntityID  int32
	ActionID  int32
	JumpBoost int32
}

func (p *PlayServerboundPlayerCommandPacket) PacketID() int32 {
	return consts.PlayServerboundPlayerCommand
}
func (p *PlayServerboundPlayerCommandPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.EntityID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.ActionID); err != nil {
		return err
	}
	return w.WriteVarInt(p.JumpBoost)
}
func (p *PlayServerboundPlayerCommandPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.ActionID, err = r.ReadVarInt(); err != nil {
		return err
	}
	p.JumpBoost, err = r.ReadVarInt()
	return err
}

// PlayServerboundSetHeldItemPacket is Set Held Item (0x2C).
type PlayServerboundSetHeldItemPacket struct {
	Slot int16
}

func (p *PlayServerboundSetHeldItemPacket) PacketID() int32 { return consts.PlayServerboundSetHeldItem }
func (p *PlayServerboundSetHeldItemPacket) Marshal(w *Writer) error {
	if p.Slot < 0 || p.Slot > 8 {
		return fmt.Errorf("invalid held item slot: %d (must be 0-8)", p.Slot)
	}
	return w.WriteShort(p.Slot)
}
func (p *PlayServerboundSetHeldItemPacket) Unmarshal(r *Reader) error {
	slot, err := r.ReadShort()
	if err != nil {
		return err
	}
	if slot < 0 || slot > 8 {
		return fmt.Errorf("invalid held item slot: %d (must be 0-8)", slot)
	}
	p.Slot = slot
	return nil
}

// PlayServerboundSetCreativeModeSlotPacket is Set Creative Mode Slot (0x2F).
type PlayServerboundSetCreativeModeSlotPacket struct {
	Slot int16
	Item ItemStack
}

func (p *PlayServerboundSetCreativeModeSlotPacket) PacketID() int32 {
	return consts.PlayServerboundSetCreativeModeSlot
}
func (p *PlayServerboundSetCreativeModeSlotPacket) Marshal(w *Writer) error {
	if err := w.WriteShort(p.Slot); err != nil {
		return err
	}
	return p.Item.Marshal(w)
}
func (p *PlayServerboundSetCreativeModeSlotPacket) Unmarshal(r *Reader) error {
	slot, err := r.ReadShort()
	if err != nil {
		return err
	}
	item, err := ReadItemStack(r)
	if err != nil {
		return err
	}
	p.Slot = slot
	p.Item = item
	return nil
}

// PlayServerboundUseItemOnPacket is Use Item On (0x35).
type PlayServerboundUseItemOnPacket struct {
	RawData     []byte
	Hand        int32
	Position    BlockPos
	Face        byte
	CursorX     float32
	CursorY     float32
	CursorZ     float32
	InsideBlock bool
	Sequence    int32
}

func (p *PlayServerboundUseItemOnPacket) PacketID() int32 { return consts.PlayServerboundUseItemOn }
func (p *PlayServerboundUseItemOnPacket) Marshal(w *Writer) error {
	if p.RawData != nil {
		_, err := w.w.Write(p.RawData)
		return err
	}
	if err := w.WriteVarInt(p.Hand); err != nil {
		return err
	}
	if err := writeBlockPos(w, p.Position); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(p.Face)); err != nil {
		return err
	}
	if err := w.WriteFloat(p.CursorX); err != nil {
		return err
	}
	if err := w.WriteFloat(p.CursorY); err != nil {
		return err
	}
	if err := w.WriteFloat(p.CursorZ); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.InsideBlock); err != nil {
		return err
	}
	return w.WriteVarInt(p.Sequence)
}
func (p *PlayServerboundUseItemOnPacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	rr := NewReader(bytes.NewReader(data))
	hand, err := rr.ReadVarInt()
	if err != nil {
		return nil
	}
	pos, err := readBlockPos(rr)
	if err != nil {
		return nil
	}
	face, err := rr.ReadVarInt()
	if err != nil {
		return nil
	}
	cursorX, err := rr.ReadFloat()
	if err != nil {
		return nil
	}
	cursorY, err := rr.ReadFloat()
	if err != nil {
		return nil
	}
	cursorZ, err := rr.ReadFloat()
	if err != nil {
		return nil
	}
	inside, err := rr.ReadBoolean()
	if err != nil {
		return nil
	}
	seq, err := rr.ReadVarInt()
	if err != nil {
		return nil
	}
	p.Hand = hand
	p.Position = pos
	p.Face = byte(face)
	p.CursorX = cursorX
	p.CursorY = cursorY
	p.CursorZ = cursorZ
	p.InsideBlock = inside
	p.Sequence = seq
	return nil
}

// PlayServerboundClientInformationPacket is Client Information (0x09).
type PlayServerboundClientInformationPacket struct {
	Data []byte
}

func (p *PlayServerboundClientInformationPacket) PacketID() int32 {
	return consts.PlayServerboundClientInformation
}
func (p *PlayServerboundClientInformationPacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.Data)
	return err
}
func (p *PlayServerboundClientInformationPacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.Data = data
	return nil
}

// PlayServerboundKeepAlivePacket is Serverbound Keep Alive (play) (0x15).
type PlayServerboundKeepAlivePacket struct {
	KeepAliveID int64
}

func (p *PlayServerboundKeepAlivePacket) PacketID() int32 {
	return consts.PlayServerboundServerboundKeepAlive
}
func (p *PlayServerboundKeepAlivePacket) Marshal(w *Writer) error { return w.WriteLong(p.KeepAliveID) }
func (p *PlayServerboundKeepAlivePacket) Unmarshal(r *Reader) error {
	id, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.KeepAliveID = id
	return nil
}

// PlayClientboundBlockUpdatePacket is Block Update (0x09).
type PlayClientboundBlockUpdatePacket struct {
	Position BlockPos
	StateID  int32
}

func (p *PlayClientboundBlockUpdatePacket) PacketID() int32 { return consts.PlayClientboundBlockUpdate }
func (p *PlayClientboundBlockUpdatePacket) Marshal(w *Writer) error {
	if err := writeBlockPos(w, p.Position); err != nil {
		return err
	}
	return w.WriteVarInt(p.StateID)
}
func (p *PlayClientboundBlockUpdatePacket) Unmarshal(r *Reader) error {
	pos, err := readBlockPos(r)
	if err != nil {
		return err
	}
	stateID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Position = pos
	p.StateID = stateID
	return nil
}

// SectionBlockChange is one local position/state pair inside Update Section Blocks.
type SectionBlockChange struct {
	LocalPos uint16
	StateID  int32
}

// PlayClientboundUpdateSectionBlocksPacket is Update Section Blocks (0x47).
type PlayClientboundUpdateSectionBlocksPacket struct {
	SectionPos ChunkSectionPos
	Changes    []SectionBlockChange
}

func (p *PlayClientboundUpdateSectionBlocksPacket) PacketID() int32 {
	return consts.PlayClientboundUpdateSectionBlocks
}
func (p *PlayClientboundUpdateSectionBlocksPacket) Marshal(w *Writer) error {
	if err := writeChunkSectionPos(w, p.SectionPos); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.Changes))); err != nil {
		return err
	}
	for _, c := range p.Changes {
		packed := (int64(c.StateID) << 12) | int64(c.LocalPos&0x0FFF)
		if err := w.WriteVarLong(packed); err != nil {
			return err
		}
	}
	return nil
}
func (p *PlayClientboundUpdateSectionBlocksPacket) Unmarshal(r *Reader) error {
	sec, err := readChunkSectionPos(r)
	if err != nil {
		return err
	}
	n, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	if n < 0 {
		return io.ErrUnexpectedEOF
	}
	changes := make([]SectionBlockChange, 0, n)
	for i := int32(0); i < n; i++ {
		v, err := r.ReadVarLong()
		if err != nil {
			return err
		}
		local := uint16(v & 0x0FFF)
		stateID := int32(v >> 12)
		changes = append(changes, SectionBlockChange{LocalPos: local, StateID: stateID})
	}
	p.SectionPos = sec
	p.Changes = changes
	return nil
}

// PlayClientboundUpdateEntityPositionAndRotationPacket is Update Entity Position and Rotation (0x2D).
type PlayClientboundUpdateEntityPositionAndRotationPacket struct {
	EntityID int32
	DX       int16
	DY       int16
	DZ       int16
	Yaw      byte
	Pitch    byte
	OnGround bool
}

func (p *PlayClientboundUpdateEntityPositionAndRotationPacket) PacketID() int32 {
	return consts.PlayClientboundUpdateEntityPositionAndRotation
}
func (p *PlayClientboundUpdateEntityPositionAndRotationPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.EntityID); err != nil {
		return err
	}
	if err := w.WriteShort(p.DX); err != nil {
		return err
	}
	if err := w.WriteShort(p.DY); err != nil {
		return err
	}
	if err := w.WriteShort(p.DZ); err != nil {
		return err
	}
	if err := w.WriteByte(p.Yaw); err != nil {
		return err
	}
	if err := w.WriteByte(p.Pitch); err != nil {
		return err
	}
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayClientboundUpdateEntityPositionAndRotationPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.DX, err = r.ReadShort(); err != nil {
		return err
	}
	if p.DY, err = r.ReadShort(); err != nil {
		return err
	}
	if p.DZ, err = r.ReadShort(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadByte(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadByte(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

// PlayClientboundUpdateTimePacket is Update Time (0x62).
type PlayClientboundUpdateTimePacket struct {
	WorldAge  int64
	TimeOfDay int64
}

func (p *PlayClientboundUpdateTimePacket) PacketID() int32 { return consts.PlayClientboundUpdateTime }
func (p *PlayClientboundUpdateTimePacket) Marshal(w *Writer) error {
	if err := w.WriteLong(p.WorldAge); err != nil {
		return err
	}
	return w.WriteLong(p.TimeOfDay)
}
func (p *PlayClientboundUpdateTimePacket) Unmarshal(r *Reader) error {
	var err error
	if p.WorldAge, err = r.ReadLong(); err != nil {
		return err
	}
	p.TimeOfDay, err = r.ReadLong()
	return err
}

const (
	PlayerInfoActionUpdateLatency byte = 1 << 4
)

type PlayClientboundPlayerInfoUpdatePlayer struct {
	UUID [16]byte
	Ping int32
}

// PlayClientboundPlayerInfoUpdatePacket is Player Info Update (0x3C).
type PlayClientboundPlayerInfoUpdatePacket struct {
	Actions byte
	Players []PlayClientboundPlayerInfoUpdatePlayer
}

func (p *PlayClientboundPlayerInfoUpdatePacket) PacketID() int32 {
	return consts.PlayClientboundPlayerInfoUpdate
}
func (p *PlayClientboundPlayerInfoUpdatePacket) Marshal(w *Writer) error {
	if err := w.WriteByte(p.Actions); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.Players))); err != nil {
		return err
	}
	for _, pl := range p.Players {
		if _, err := w.w.Write(pl.UUID[:]); err != nil {
			return err
		}
		if p.Actions&PlayerInfoActionUpdateLatency != 0 {
			if err := w.WriteVarInt(pl.Ping); err != nil {
				return err
			}
		}
	}
	return nil
}
func (p *PlayClientboundPlayerInfoUpdatePacket) Unmarshal(r *Reader) error {
	actions, err := r.ReadByte()
	if err != nil {
		return err
	}
	n, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	if n < 0 {
		return io.ErrUnexpectedEOF
	}
	players := make([]PlayClientboundPlayerInfoUpdatePlayer, 0, n)
	for i := int32(0); i < n; i++ {
		u, err := r.ReadUUID()
		if err != nil {
			return err
		}
		player := PlayClientboundPlayerInfoUpdatePlayer{UUID: u}
		if actions&PlayerInfoActionUpdateLatency != 0 {
			ping, err := r.ReadVarInt()
			if err != nil {
				return err
			}
			player.Ping = ping
		}
		players = append(players, player)
	}
	p.Actions = actions
	p.Players = players
	return nil
}

// PlayClientboundUpdateEntityPositionPacket is Update Entity Position (0x2C).
type PlayClientboundUpdateEntityPositionPacket struct {
	EntityID int32
	DX       int16
	DY       int16
	DZ       int16
	OnGround bool
}

func (p *PlayClientboundUpdateEntityPositionPacket) PacketID() int32 {
	return consts.PlayClientboundUpdateEntityPosition
}
func (p *PlayClientboundUpdateEntityPositionPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(p.EntityID)
	_ = w.WriteShort(p.DX)
	_ = w.WriteShort(p.DY)
	_ = w.WriteShort(p.DZ)
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayClientboundUpdateEntityPositionPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.DX, err = r.ReadShort(); err != nil {
		return err
	}
	if p.DY, err = r.ReadShort(); err != nil {
		return err
	}
	if p.DZ, err = r.ReadShort(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

// PlayClientboundUpdateEntityRotationPacket is Update Entity Rotation (0x2E).
type PlayClientboundUpdateEntityRotationPacket struct {
	EntityID int32
	Yaw      byte
	Pitch    byte
	OnGround bool
}

func (p *PlayClientboundUpdateEntityRotationPacket) PacketID() int32 {
	return consts.PlayClientboundUpdateEntityRotation
}
func (p *PlayClientboundUpdateEntityRotationPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(p.EntityID)
	_ = w.WriteByte(p.Yaw)
	_ = w.WriteByte(p.Pitch)
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayClientboundUpdateEntityRotationPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadByte(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadByte(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

// PlayClientboundSetEntityVelocityPacket is Set Entity Velocity (0x58).
type PlayClientboundSetEntityVelocityPacket struct {
	EntityID  int32
	VelocityX int16
	VelocityY int16
	VelocityZ int16
}

func (p *PlayClientboundSetEntityVelocityPacket) PacketID() int32 {
	return consts.PlayClientboundSetEntityVelocity
}
func (p *PlayClientboundSetEntityVelocityPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(p.EntityID)
	_ = w.WriteShort(p.VelocityX)
	_ = w.WriteShort(p.VelocityY)
	return w.WriteShort(p.VelocityZ)
}
func (p *PlayClientboundSetEntityVelocityPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.VelocityX, err = r.ReadShort(); err != nil {
		return err
	}
	if p.VelocityY, err = r.ReadShort(); err != nil {
		return err
	}
	p.VelocityZ, err = r.ReadShort()
	return err
}

// PlayClientboundTeleportEntityPacket is Teleport Entity (0x6D).
type PlayClientboundTeleportEntityPacket struct {
	EntityID int32
	X        float64
	Y        float64
	Z        float64
	Yaw      byte
	Pitch    byte
	OnGround bool
}

func (p *PlayClientboundTeleportEntityPacket) PacketID() int32 {
	return consts.PlayClientboundTeleportEntity
}
func (p *PlayClientboundTeleportEntityPacket) Marshal(w *Writer) error {
	_ = w.WriteVarInt(p.EntityID)
	_ = w.WriteDouble(p.X)
	_ = w.WriteDouble(p.Y)
	_ = w.WriteDouble(p.Z)
	_ = w.WriteByte(p.Yaw)
	_ = w.WriteByte(p.Pitch)
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayClientboundTeleportEntityPacket) Unmarshal(r *Reader) error {
	var err error
	if p.EntityID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadByte(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadByte(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

func unmarshalPacketBody(p Packet, data []byte) error {
	return p.Unmarshal(NewReader(bytes.NewReader(data)))
}

// PlayServerboundSwingArmPacket is Swing Arm (0x33).
type PlayServerboundSwingArmPacket struct {
	Hand int32
}

func (p *PlayServerboundSwingArmPacket) PacketID() int32 {
	return consts.PlayServerboundSwingArm
}

func (p *PlayServerboundSwingArmPacket) Marshal(w *Writer) error {
	return w.WriteVarInt(p.Hand)
}

func (p *PlayServerboundSwingArmPacket) Unmarshal(r *Reader) error {
	hand, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Hand = hand
	return nil
}

type EntityPose string

const (
	PoseStanding   EntityPose = "standing"
	PoseSneaking   EntityPose = "sneaking"
	PoseSwimming   EntityPose = "swimming"
	PoseFallFlying EntityPose = "fall_flying"
	PoseSleeping   EntityPose = "sleeping"
	PoseCrawling   EntityPose = "crawling"
)

type EntityMetadataEntry struct {
	Index    byte
	TypeID   int32
	Value    any
	RawValue []byte
	Known    bool
}

// PlayClientboundSetEntityMetadataPacket is Set Entity Metadata (0x56).
type PlayClientboundSetEntityMetadataPacket struct {
	EntityID int32
	Metadata []EntityMetadataEntry
}

func (p *PlayClientboundSetEntityMetadataPacket) PacketID() int32 {
	return consts.PlayClientboundSetEntityMetadata
}

func (p *PlayClientboundSetEntityMetadataPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.EntityID); err != nil {
		return err
	}
	for _, entry := range p.Metadata {
		if err := w.WriteByte(entry.Index); err != nil {
			return err
		}
		if err := w.WriteVarInt(entry.TypeID); err != nil {
			return err
		}
		if _, err := w.w.Write(entry.RawValue); err != nil {
			return err
		}
	}
	return w.WriteByte(0xFF) // End marker
}

func (p *PlayClientboundSetEntityMetadataPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.EntityID = id
	metadata, err := ReadEntityMetadata(r)
	if err != nil {
		return err
	}
	p.Metadata = metadata
	return nil
}

func ReadEntityMetadata(r *Reader) ([]EntityMetadataEntry, error) {
	var entries []EntityMetadataEntry
	for {
		index, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if index == 0xFF {
			break
		}

		typeID, err := r.ReadVarInt()
		if err != nil {
			return nil, err
		}

		entry := EntityMetadataEntry{
			Index:  index,
			TypeID: typeID,
		}

		tr := &trackingReader{r: r.r}
		dec := NewReader(tr)

		value, err := parseMetadataValue(dec, typeID)
		if err != nil {
			return nil, err
		}
		entry.Value = value
		entry.RawValue = tr.buf.Bytes()
		entry.Known = true
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseMetadataValue(r *Reader, typeID int32) (any, error) {
	switch typeID {
	case 0: // Byte
		return r.ReadByte()
	case 1: // VarInt
		return r.ReadVarInt()
	case 2: // VarLong
		return readVarLong(r)
	case 3: // Float
		return r.ReadFloat()
	case 4: // String
		return r.ReadString()
	case 5: // Chat
		return readNBTBytes(r)
	case 6: // OptChat
		present, err := r.ReadBoolean()
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, nil
		}
		return readNBTBytes(r)
	case 7: // Slot
		return ReadItemStack(r)
	case 8: // Boolean
		return r.ReadBoolean()
	case 9: // Rotations (3 floats)
		x, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		y, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		z, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		return [3]float32{x, y, z}, nil
	case 10: // Position
		return readBlockPos(r)
	case 11: // OptPosition
		present, err := r.ReadBoolean()
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, nil
		}
		return readBlockPos(r)
	case 12: // Direction (VarInt)
		return r.ReadVarInt()
	case 13: // OptUUID
		present, err := r.ReadBoolean()
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, nil
		}
		return r.ReadUUID()
	case 14: // BlockState (VarInt)
		return r.ReadVarInt()
	case 15: // OptBlockState (VarInt)
		return r.ReadVarInt()
	case 16: // NBT Compound
		return readNBTBytes(r)
	case 17: // Particle (VarInt ID + basic skip payload logic for test variant safety)
		id, err := r.ReadVarInt()
		if err != nil {
			return nil, err
		}
		// In Minecraft 1.20.4 (Protocol 765), some particles have extra fields:
		// block (2), block_marker (3), falling_dust (24) -> VarInt (Block State ID)
		// dust (14) -> 4 Floats (Red, Green, Blue, Scale)
		// dust_color_transition (15) -> 7 Floats (fromR, fromG, fromB, scale, toR, toG, toB)
		// item (35) -> Slot (ItemStack)
		// sculk_charge (54) -> Float (roll angle)
		// shriek (57) -> VarInt (delay)
		// vibration (60) -> Position Source (VarInt type: 0=Block, 1=Entity) + VarInt (ticks)
		//   if Block (0): BlockPos
		//   if Entity (1): VarInt (Entity ID) + Float (Eye Height)
		switch id {
		case 2, 3, 24:
			if _, err := r.ReadVarInt(); err != nil {
				return nil, err
			}
		case 14:
			for i := 0; i < 4; i++ {
				if _, err := r.ReadFloat(); err != nil {
					return nil, err
				}
			}
		case 15:
			for i := 0; i < 7; i++ {
				if _, err := r.ReadFloat(); err != nil {
					return nil, err
				}
			}
		case 35:
			if _, err := ReadItemStack(r); err != nil {
				return nil, err
			}
		case 54:
			if _, err := r.ReadFloat(); err != nil {
				return nil, err
			}
		case 57:
			if _, err := r.ReadVarInt(); err != nil {
				return nil, err
			}
		case 60:
			srcType, err := r.ReadVarInt()
			if err != nil {
				return nil, err
			}
			if srcType == 0 { // Block
				if _, err := readBlockPos(r); err != nil {
					return nil, err
				}
			} else if srcType == 1 { // Entity
				if _, err := r.ReadVarInt(); err != nil {
					return nil, err
				}
				if _, err := r.ReadFloat(); err != nil {
					return nil, err
				}
			}
			if _, err := r.ReadVarInt(); err != nil {
				return nil, err
			}
		}
		return id, nil
	case 18: // Villager Data
		typeVal, err := r.ReadVarInt()
		if err != nil {
			return nil, err
		}
		profession, err := r.ReadVarInt()
		if err != nil {
			return nil, err
		}
		level, err := r.ReadVarInt()
		if err != nil {
			return nil, err
		}
		return [3]int32{typeVal, profession, level}, nil
	case 19: // OptVarInt
		present, err := r.ReadBoolean()
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, nil
		}
		return r.ReadVarInt()
	case 20: // Pose
		return r.ReadVarInt()
	case 21, 22, 23, 24, 25: // Variants
		return r.ReadVarInt()
	case 26: // Vector3
		x, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		y, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		z, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		return [3]float32{x, y, z}, nil
	case 27: // Quaternion
		x, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		y, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		z, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		wVal, err := r.ReadFloat()
		if err != nil {
			return nil, err
		}
		return [4]float32{x, y, z, wVal}, nil
	default:
		return nil, fmt.Errorf("unsupported metadata type: %d", typeID)
	}
}

func readNBTBytes(r *Reader) ([]byte, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if tag == 0 {
		return []byte{0x00}, nil
	}
	tr := &trackingReader{r: r.r}
	tr.buf.WriteByte(tag)
	trackingDec := NewReader(tr)
	if err := skipNBTPayload(trackingDec, tag); err != nil {
		return nil, err
	}
	return tr.buf.Bytes(), nil
}

func readVarLong(r *Reader) (int64, error) {
	var value int64
	var position int
	for {
		currentByte, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int64(currentByte&0x7F) << position
		if (currentByte & 0x80) == 0 {
			break
		}
		position += 7
		if position >= 64 {
			return 0, fmt.Errorf("VarLong is too big")
		}
	}
	return value, nil
}

// PlayClientboundOpenScreenPacket is Open Screen (0x31).
type PlayClientboundOpenScreenPacket struct {
	WindowID   int32
	WindowType int32
	Title      string
}

func (p *PlayClientboundOpenScreenPacket) PacketID() int32 {
	return consts.PlayClientboundOpenScreen
}
func (p *PlayClientboundOpenScreenPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.WindowID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.WindowType); err != nil {
		return err
	}
	return w.WriteString(p.Title)
}
func (p *PlayClientboundOpenScreenPacket) Unmarshal(r *Reader) error {
	var err error
	if p.WindowID, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.WindowType, err = r.ReadVarInt(); err != nil {
		return err
	}
	p.Title, err = r.ReadString()
	return err
}

// PlayClientboundCloseContainerPacket is Close Container (clientbound) (0x12).
type PlayClientboundCloseContainerPacket struct {
	WindowID byte
}

func (p *PlayClientboundCloseContainerPacket) PacketID() int32 {
	return consts.PlayClientboundCloseContainer
}
func (p *PlayClientboundCloseContainerPacket) Marshal(w *Writer) error {
	return w.WriteByte(p.WindowID)
}
func (p *PlayClientboundCloseContainerPacket) Unmarshal(r *Reader) error {
	var err error
	p.WindowID, err = r.ReadByte()
	return err
}

// PlayServerboundCloseContainerPacket is Close Container (serverbound) (0x0E).
type PlayServerboundCloseContainerPacket struct {
	WindowID byte
}

func (p *PlayServerboundCloseContainerPacket) PacketID() int32 {
	return consts.PlayServerboundCloseContainer
}
func (p *PlayServerboundCloseContainerPacket) Marshal(w *Writer) error {
	return w.WriteByte(p.WindowID)
}
func (p *PlayServerboundCloseContainerPacket) Unmarshal(r *Reader) error {
	var err error
	p.WindowID, err = r.ReadByte()
	return err
}

// --- Protocol 765 (1.20.4) play-state reliability packets ---------------------
//
// The three clientbound packets below were verified against PrismarineJS
// minecraft-data "pc/1.20.3" (the data set 1.20.4 shares; protocol 765), which
// node-minecraft-protocol / mineflayer use against live 1.20.4 servers. Packet
// IDs match pkg/protocol/consts/play.go exactly.

// PlayClientboundBundleDelimiterPacket is Bundle Delimiter (clientbound, 0x00).
//
// Protocol 765: the server brackets a group of packets that must be applied in
// the same client tick with two Bundle Delimiters (one before, one after). The
// packet itself has NO payload (minecraft-data models it as "void"). FeastGo's
// reader already delivers packets one at a time in wire order, so the delimiter
// is a non-fatal marker only; the packets inside a bundle dispatch normally and
// in the order received.
type PlayClientboundBundleDelimiterPacket struct{}

func (p *PlayClientboundBundleDelimiterPacket) PacketID() int32 {
	return consts.PlayClientboundBundleDelimiter
}

// Marshal writes nothing: a Bundle Delimiter is a zero-length body.
func (p *PlayClientboundBundleDelimiterPacket) Marshal(w *Writer) error { return nil }

// Unmarshal enforces the zero-length body. It reads a single byte and requires
// EOF; any trailing byte means the frame was mis-identified and decoding it as a
// delimiter would desync the stream. Probing one byte (rather than draining the
// reader with io.ReadAll) keeps the common empty-payload path allocation-free.
func (p *PlayClientboundBundleDelimiterPacket) Unmarshal(r *Reader) error {
	if _, err := r.ReadByte(); err == nil {
		return fmt.Errorf("protocol: bundle delimiter expects zero payload")
	} else if err != io.EOF {
		return err
	}
	return nil
}

// PlayClientboundAcknowledgeBlockChangePacket is Acknowledge Block Change
// (clientbound, 0x05).
//
// Protocol 765: sent by the SERVER to acknowledge a block-change sequence the
// client previously sent in the trailing "Sequence" VarInt of Player Action
// (0x21), Use Item On (0x35) or Use Item (0x36). It carries a single VarInt
// sequence id. There is NO serverbound Acknowledge Block Change packet in 765;
// the serverbound half of this handshake is the per-action Sequence field that
// PlayServerboundPlayerActionPacket and PlayServerboundUseItemOnPacket already
// emit.
//
// An acknowledgement only tells the client the server has processed up to that
// sequence so it may stop predicting it; it does NOT by itself mean a placement
// or break succeeded. The authoritative result is still the Block Update (0x09)
// / Update Section Blocks (0x47) the server sends for the affected block(s).
// (minecraft-data names this packet "acknowledge_player_digging".)
type PlayClientboundAcknowledgeBlockChangePacket struct {
	SequenceID int32
}

func (p *PlayClientboundAcknowledgeBlockChangePacket) PacketID() int32 {
	return consts.PlayClientboundAcknowledgeBlockChange
}
func (p *PlayClientboundAcknowledgeBlockChangePacket) Marshal(w *Writer) error {
	return w.WriteVarInt(p.SequenceID)
}
func (p *PlayClientboundAcknowledgeBlockChangePacket) Unmarshal(r *Reader) error {
	seq, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.SequenceID = seq
	return nil
}

// PlayClientboundRespawnPacket is Respawn (clientbound, 0x45).
//
// Protocol 765 field layout (verified against minecraft-data pc/1.20.3):
//
//	Dimension Type      Identifier (String)  registry key of the dimension type
//	Dimension Name      Identifier (String)  the world/dimension being entered
//	Hashed Seed         Long
//	Game Mode           Unsigned Byte        0 survival,1 creative,2 adventure,3 spectator
//	Previous Game Mode  Byte                 -1 if none
//	Is Debug            Boolean
//	Is Flat             Boolean
//	Has Death Location  Boolean
//	  Death Dimension   Identifier (String)  only if Has Death Location
//	  Death Location    Position             only if Has Death Location
//	Portal Cooldown     VarInt
//	Data Kept           Unsigned Byte        bitmask: 0x01 keep attributes, 0x02 keep metadata
//
// "Data Kept" is a single byte on the wire (minecraft-data models it as the
// boolean "copyMetadata"); it is decoded verbatim as a bitmask so no information
// is lost. The dimension type/name are 1.20.2+ string Identifiers, not the
// VarInt registry ids introduced in 1.20.5 / protocol 766.
type PlayClientboundRespawnPacket struct {
	DimensionType    string
	DimensionName    string
	HashedSeed       int64
	GameMode         byte
	PreviousGameMode byte
	IsDebug          bool
	IsFlat           bool
	HasDeathLocation bool
	DeathDimension   string
	DeathLocation    BlockPos
	PortalCooldown   int32
	DataKept         byte
}

func (p *PlayClientboundRespawnPacket) PacketID() int32 { return consts.PlayClientboundRespawn }

func (p *PlayClientboundRespawnPacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.DimensionType); err != nil {
		return err
	}
	if err := w.WriteString(p.DimensionName); err != nil {
		return err
	}
	if err := w.WriteLong(p.HashedSeed); err != nil {
		return err
	}
	if err := w.WriteByte(p.GameMode); err != nil {
		return err
	}
	if err := w.WriteByte(p.PreviousGameMode); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.IsDebug); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.IsFlat); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.HasDeathLocation); err != nil {
		return err
	}
	if p.HasDeathLocation {
		if err := w.WriteString(p.DeathDimension); err != nil {
			return err
		}
		if err := writeBlockPos(w, p.DeathLocation); err != nil {
			return err
		}
	}
	if err := w.WriteVarInt(p.PortalCooldown); err != nil {
		return err
	}
	return w.WriteByte(p.DataKept)
}

func (p *PlayClientboundRespawnPacket) Unmarshal(r *Reader) error {
	var err error
	if p.DimensionType, err = r.ReadString(); err != nil {
		return err
	}
	if p.DimensionName, err = r.ReadString(); err != nil {
		return err
	}
	if p.HashedSeed, err = r.ReadLong(); err != nil {
		return err
	}
	if p.GameMode, err = r.ReadByte(); err != nil {
		return err
	}
	if p.PreviousGameMode, err = r.ReadByte(); err != nil {
		return err
	}
	if p.IsDebug, err = r.ReadBoolean(); err != nil {
		return err
	}
	if p.IsFlat, err = r.ReadBoolean(); err != nil {
		return err
	}
	if p.HasDeathLocation, err = r.ReadBoolean(); err != nil {
		return err
	}
	if p.HasDeathLocation {
		if p.DeathDimension, err = r.ReadString(); err != nil {
			return err
		}
		if p.DeathLocation, err = readBlockPos(r); err != nil {
			return err
		}
	}
	if p.PortalCooldown, err = r.ReadVarInt(); err != nil {
		return err
	}
	if p.DataKept, err = r.ReadByte(); err != nil {
		return err
	}
	return nil
}

// CopyMetadata reports whether the server asked the client to keep entity
// metadata across this respawn (Data Kept bit 0x02). FeastGo does not act on it
// today, but the decoded value is surfaced for callers that need it.
func (p *PlayClientboundRespawnPacket) CopyMetadata() bool {
	return p.DataKept&0x02 != 0
}
