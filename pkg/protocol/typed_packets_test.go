package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func roundTripPacket(t *testing.T, p Packet, clone func() Packet) {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := p.Marshal(w); err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	r := NewReader(bytes.NewReader(buf.Bytes()))
	decoded := clone()
	if err := decoded.Unmarshal(r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	var buf2 bytes.Buffer
	w2 := NewWriter(&buf2)
	if err := decoded.Marshal(w2); err != nil {
		t.Fatalf("re-marshal failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), buf2.Bytes()) {
		t.Fatalf("roundtrip mismatch\norig=%x\nout=%x", buf.Bytes(), buf2.Bytes())
	}
}

func TestLoginPacketsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		pkt   Packet
		clone func() Packet
	}{
		{"disconnect_login", &LoginClientboundDisconnectPacket{Reason: "bye"}, func() Packet { return &LoginClientboundDisconnectPacket{} }},
		{"encryption_request", &LoginClientboundEncryptionRequestPacket{ServerID: "", PublicKey: []byte{1, 2, 3}, VerifyToken: []byte{4, 5, 6, 7}}, func() Packet { return &LoginClientboundEncryptionRequestPacket{} }},
		{"login_success", &LoginClientboundLoginSuccessPacket{UUID: [16]byte{1, 2}, Username: "FeastBot", Properties: []LoginProperty{{Name: "textures", Value: "abc", Signature: "sig", IsSigned: true}}}, func() Packet { return &LoginClientboundLoginSuccessPacket{} }},
		{"set_compression", &LoginClientboundSetCompressionPacket{Threshold: 256}, func() Packet { return &LoginClientboundSetCompressionPacket{} }},
		{"login_plugin_request", &LoginClientboundPluginRequestPacket{MessageID: 9, Channel: "fml:handshake", Data: []byte{9, 8, 7}}, func() Packet { return &LoginClientboundPluginRequestPacket{} }},
		{"login_start", &LoginServerboundStartPacket{Username: "FeastBot", UUID: [16]byte{9, 9}}, func() Packet { return &LoginServerboundStartPacket{} }},
		{"encryption_response", &LoginServerboundEncryptionResponsePacket{SharedSecret: []byte{1, 2, 3}, VerifyToken: []byte{4, 5}}, func() Packet { return &LoginServerboundEncryptionResponsePacket{} }},
		{"login_plugin_response", &LoginServerboundPluginResponsePacket{MessageID: 3, Successful: true, Data: []byte{1, 0, 1}}, func() Packet { return &LoginServerboundPluginResponsePacket{} }},
		{"login_ack", &LoginServerboundAcknowledgedPacket{}, func() Packet { return &LoginServerboundAcknowledgedPacket{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripPacket(t, tc.pkt, tc.clone)
		})
	}
}

func TestConfigurationPacketsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		pkt   Packet
		clone func() Packet
	}{
		{"cb_plugin_msg", &ConfigClientboundPluginMessagePacket{Channel: "minecraft:brand", Data: []byte("vanilla")}, func() Packet { return &ConfigClientboundPluginMessagePacket{} }},
		{"cb_disconnect", &ConfigClientboundDisconnectPacket{Reason: "bye"}, func() Packet { return &ConfigClientboundDisconnectPacket{} }},
		{"cb_finish", &ConfigClientboundFinishPacket{}, func() Packet { return &ConfigClientboundFinishPacket{} }},
		{"cb_keepalive", &ConfigClientboundKeepAlivePacket{KeepAliveID: 42}, func() Packet { return &ConfigClientboundKeepAlivePacket{} }},
		{"cb_ping", &ConfigClientboundPingPacket{ID: 11}, func() Packet { return &ConfigClientboundPingPacket{} }},
		{"cb_registry_data", &ConfigClientboundRegistryDataPacket{RegistryID: "minecraft:dimension_type", Data: []byte{0x0A, 0x00, 0x00}}, func() Packet { return &ConfigClientboundRegistryDataPacket{} }},
		{"cb_remove_resource_pack", &ConfigClientboundRemoveResourcePackPacket{HasUUID: true, UUID: [16]byte{1, 1}}, func() Packet { return &ConfigClientboundRemoveResourcePackPacket{} }},
		{"cb_add_resource_pack", &ConfigClientboundAddResourcePackPacket{UUID: [16]byte{2, 2}, URL: "https://example.com/pack.zip", Hash: "abcd", Forced: true, HasPromptMessage: true, PromptMessage: "Use this pack"}, func() Packet { return &ConfigClientboundAddResourcePackPacket{} }},
		{"cb_feature_flags", &ConfigClientboundFeatureFlagsPacket{Flags: []string{"minecraft:vanilla"}}, func() Packet { return &ConfigClientboundFeatureFlagsPacket{} }},
		{"cb_update_tags", &ConfigClientboundUpdateTagsPacket{Data: []byte{1, 2, 3}}, func() Packet { return &ConfigClientboundUpdateTagsPacket{} }},
		{"sb_client_info", &ConfigServerboundClientInformationPacket{Locale: "en_us", ViewDistance: 10, ChatMode: 0, ChatColors: true, DisplayedSkinParts: 0x7f, MainHand: 1, EnableTextFiltering: false, AllowServerListings: true}, func() Packet { return &ConfigServerboundClientInformationPacket{} }},
		{"sb_plugin_msg", &ConfigServerboundPluginMessagePacket{Channel: "minecraft:brand", Data: []byte("bot")}, func() Packet { return &ConfigServerboundPluginMessagePacket{} }},
		{"sb_ack_finish", &ConfigServerboundAcknowledgeFinishPacket{}, func() Packet { return &ConfigServerboundAcknowledgeFinishPacket{} }},
		{"sb_keepalive", &ConfigServerboundKeepAlivePacket{KeepAliveID: 99}, func() Packet { return &ConfigServerboundKeepAlivePacket{} }},
		{"sb_pong", &ConfigServerboundPongPacket{ID: 12}, func() Packet { return &ConfigServerboundPongPacket{} }},
		{"sb_resource_pack_resp", &ConfigServerboundResourcePackResponsePacket{UUID: [16]byte{3, 3}, Result: 2}, func() Packet { return &ConfigServerboundResourcePackResponsePacket{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripPacket(t, tc.pkt, tc.clone)
		})
	}
}

func TestPlayPacketsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		pkt   Packet
		clone func() Packet
	}{
		{"cb_login_play", &PlayClientboundLoginPacket{EntityID: 7, TailData: []byte{1, 2}}, func() Packet { return &PlayClientboundLoginPacket{} }},
		{"cb_sync_pos", &PlayClientboundSynchronizePlayerPositionPacket{X: 1.25, Y: 64, Z: -2.5, Yaw: 90, Pitch: 10, Flags: 0x1F, TeleportID: 12, TailData: []byte{9}}, func() Packet { return &PlayClientboundSynchronizePlayerPositionPacket{} }},
		{"cb_system_chat", &PlayClientboundSystemChatMessagePacket{Message: "{\"text\":\"hi\"}", Overlay: false}, func() Packet { return &PlayClientboundSystemChatMessagePacket{} }},
		{"cb_player_chat", &PlayClientboundPlayerChatMessagePacket{RawData: []byte{0x01, 0x02, 0x03}}, func() Packet { return &PlayClientboundPlayerChatMessagePacket{} }},
		{"cb_keepalive", &PlayClientboundKeepAlivePacket{KeepAliveID: 12345}, func() Packet { return &PlayClientboundKeepAlivePacket{} }},
		{"cb_disconnect", &PlayClientboundDisconnectPacket{Reason: "bye"}, func() Packet { return &PlayClientboundDisconnectPacket{} }},
		{"cb_chunk_data_light", &PlayClientboundChunkDataAndUpdateLightPacket{ChunkX: 10, ChunkZ: -3, TailData: []byte{0xAA, 0xBB}}, func() Packet { return &PlayClientboundChunkDataAndUpdateLightPacket{} }},
		{"cb_set_health", &PlayClientboundSetHealthPacket{Health: 18.5, Food: 20, Saturation: 5.0, AdditionalData: []byte{0x00}}, func() Packet { return &PlayClientboundSetHealthPacket{} }},
		{"cb_set_container_content", &PlayClientboundSetContainerContentPacket{WindowID: 1, StateID: 2, SlotCount: 3, RawSlotData: []byte{0x10, 0x20}}, func() Packet { return &PlayClientboundSetContainerContentPacket{} }},
		{"cb_set_container_slot", &PlayClientboundSetContainerSlotPacket{WindowID: 0, StateID: 2, Slot: 36, Item: ItemStack{Present: true, ItemID: 1, Count: 64, NBT: []byte{0x00}}}, func() Packet { return &PlayClientboundSetContainerSlotPacket{} }},
		{"cb_set_held_item", &PlayClientboundSetHeldItemPacket{Slot: 3}, func() Packet { return &PlayClientboundSetHeldItemPacket{} }},
		{"sb_confirm_teleport", &PlayServerboundConfirmTeleportationPacket{TeleportID: 77}, func() Packet { return &PlayServerboundConfirmTeleportationPacket{} }},
		{"sb_chat_message", &PlayServerboundChatMessagePacket{Message: "hello", Timestamp: 1, Salt: 2, MessageCount: 3}, func() Packet { return &PlayServerboundChatMessagePacket{} }},
		{"sb_set_pos_rot", &PlayServerboundSetPlayerPositionAndRotationPacket{X: 1, Y: 2, Z: 3, Yaw: 4, Pitch: 5, OnGround: true}, func() Packet { return &PlayServerboundSetPlayerPositionAndRotationPacket{} }},
		{"sb_set_held_item", &PlayServerboundSetHeldItemPacket{Slot: 3}, func() Packet { return &PlayServerboundSetHeldItemPacket{} }},
		{"sb_creative_slot", &PlayServerboundSetCreativeModeSlotPacket{Slot: 36, Item: ItemStack{Present: true, ItemID: 1, Count: 64, NBT: []byte{0x00}}}, func() Packet { return &PlayServerboundSetCreativeModeSlotPacket{} }},
		{"sb_player_action", &PlayServerboundPlayerActionPacket{RawData: []byte{0x01, 0x02}}, func() Packet { return &PlayServerboundPlayerActionPacket{} }},
		{"sb_use_item_on", &PlayServerboundUseItemOnPacket{Hand: MainHand, Position: BlockPos{X: -5, Y: 64, Z: 7}, Face: BlockFaceTop, CursorX: 0.5, CursorY: 1, CursorZ: 0.5, Sequence: 9}, func() Packet { return &PlayServerboundUseItemOnPacket{} }},
		{"sb_keepalive", &PlayServerboundKeepAlivePacket{KeepAliveID: 98765}, func() Packet { return &PlayServerboundKeepAlivePacket{} }},
		{"cb_spawn_entity", &PlayClientboundSpawnEntityPacket{EntityID: 5, Type: 1, X: 1.5, Y: 2.5, Z: 3.5, Yaw: 10, Pitch: 20, HeadYaw: 30, Data: 5, VelocityX: 10, VelocityY: 20, VelocityZ: 30}, func() Packet { return &PlayClientboundSpawnEntityPacket{} }},
		{"cb_update_entity_pos", &PlayClientboundUpdateEntityPositionPacket{EntityID: 5, DX: 100, DY: 200, DZ: 300, OnGround: true}, func() Packet { return &PlayClientboundUpdateEntityPositionPacket{} }},
		{"cb_update_entity_pos_rot", &PlayClientboundUpdateEntityPositionAndRotationPacket{EntityID: 5, DX: 100, DY: 200, DZ: 300, Yaw: 128, Pitch: 64, OnGround: true}, func() Packet { return &PlayClientboundUpdateEntityPositionAndRotationPacket{} }},
		{"cb_update_entity_rot", &PlayClientboundUpdateEntityRotationPacket{EntityID: 5, Yaw: 128, Pitch: 64, OnGround: true}, func() Packet { return &PlayClientboundUpdateEntityRotationPacket{} }},
		{"cb_set_entity_vel", &PlayClientboundSetEntityVelocityPacket{EntityID: 5, VelocityX: 100, VelocityY: 200, VelocityZ: 300}, func() Packet { return &PlayClientboundSetEntityVelocityPacket{} }},
		{"cb_teleport_entity", &PlayClientboundTeleportEntityPacket{EntityID: 5, X: 10.5, Y: 20.5, Z: 30.5, Yaw: 128, Pitch: 64, OnGround: true}, func() Packet { return &PlayClientboundTeleportEntityPacket{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripPacket(t, tc.pkt, tc.clone)
		})
	}
}

func TestClientInformationExactBytes(t *testing.T) {
	pkt := &ConfigServerboundClientInformationPacket{
		Locale:              "en_us",
		ViewDistance:        10,
		ChatMode:            0,
		ChatColors:          true,
		DisplayedSkinParts:  0x7f,
		MainHand:            1,
		EnableTextFiltering: false,
		AllowServerListings: true,
	}

	var buf bytes.Buffer
	w := NewWriter(&buf)
	err := pkt.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	out := buf.Bytes()
	// Expected layout:
	// String length (VarInt) = 5
	// Locale bytes = "en_us" (5 bytes)
	// ViewDistance = 10 (0x0A)
	// ChatMode (VarInt) = 0
	// ChatColors (Bool) = true (0x01)
	// DisplayedSkinParts (Byte) = 0x7F
	// MainHand (VarInt) = 1
	// EnableTextFiltering (Bool) = false (0x00)
	// AllowServerListings (Bool) = true (0x01)

	expected := []byte{
		0x05, 0x65, 0x6e, 0x5f, 0x75, 0x73, // "en_us"
		0x0a, // 10
		0x00, // 0
		0x01, // true
		0x7f, // 0x7F
		0x01, // 1
		0x00, // false
		0x01, // true
	}

	if !bytes.Equal(out, expected) {
		t.Fatalf("ClientInformation encoding mismatch\ngot : %x\nwant: %x", out, expected)
	}
}

func TestPlayerChatMessageUnmarshalExtractsPlainMessage(t *testing.T) {
	var body bytes.Buffer
	w := NewWriter(&body)
	if err := w.WriteUUID([16]byte{1, 2, 3}); err != nil {
		t.Fatalf("write uuid: %v", err)
	}
	if err := w.WriteVarInt(0); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := w.WriteBoolean(false); err != nil {
		t.Fatalf("write no signature: %v", err)
	}
	if err := w.WriteString("hello from FeastGo full integration test"); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	if err := w.WriteLong(123); err != nil {
		t.Fatalf("write timestamp: %v", err)
	}
	if err := w.WriteLong(456); err != nil {
		t.Fatalf("write salt: %v", err)
	}
	if err := w.WriteVarInt(0); err != nil {
		t.Fatalf("write previous message count: %v", err)
	}
	if err := w.WriteBoolean(false); err != nil {
		t.Fatalf("write no unsigned content: %v", err)
	}
	if err := w.WriteVarInt(0); err != nil {
		t.Fatalf("write filter type: %v", err)
	}
	if err := w.WriteVarInt(1); err != nil {
		t.Fatalf("write chat type: %v", err)
	}
	if err := w.WriteString(`{"text":"FeastGoBot"}`); err != nil {
		t.Fatalf("write sender display name: %v", err)
	}
	if err := w.WriteBoolean(false); err != nil {
		t.Fatalf("write no target name: %v", err)
	}

	pkt := &PlayClientboundPlayerChatMessagePacket{}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(body.Bytes()))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pkt.PlainMessage != "hello from FeastGo full integration test" {
		t.Fatalf("plain message=%q", pkt.PlainMessage)
	}
	if strings.Contains(pkt.DisplayMessage(), "\ufffd") {
		t.Fatalf("display contains replacement character: %q", pkt.DisplayMessage())
	}
	if pkt.DisplayMessage() != "FeastGoBot: hello from FeastGo full integration test" {
		t.Fatalf("display=%q", pkt.DisplayMessage())
	}
}

func TestSystemChatMessageDisplayTextStripsSimpleJSONComponent(t *testing.T) {
	pkt := &PlayClientboundSystemChatMessagePacket{Message: `{"text":"hello from FeastGo full integration test"}`}
	if got := pkt.DisplayText(); got != "hello from FeastGo full integration test" {
		t.Fatalf("DisplayText()=%q", got)
	}
}

func TestSystemChatTranslateWithParams(t *testing.T) {
	pkt := &PlayClientboundSystemChatMessagePacket{
		Message: `{"translate":"multiplayer.player.joined","with":[{"text":"FeastGoBot"}]}`,
	}
	want := "multiplayer.player.joined FeastGoBot"
	if got := pkt.DisplayText(); got != want {
		t.Fatalf("DisplayText()=%q, want %q", got, want)
	}
}

func TestSystemChatNestedTextExtra(t *testing.T) {
	pkt := &PlayClientboundSystemChatMessagePacket{
		Message: `{"text":"","extra":[{"text":"Hello "},{"text":"World"}]}`,
	}
	want := "Hello World"
	if got := pkt.DisplayText(); got != want {
		t.Fatalf("DisplayText()=%q, want %q", got, want)
	}
}

func TestSystemChatTranslateNoWith(t *testing.T) {
	pkt := &PlayClientboundSystemChatMessagePacket{
		Message: `{"translate":"commands.help.failed"}`,
	}
	want := "commands.help.failed"
	if got := pkt.DisplayText(); got != want {
		t.Fatalf("DisplayText()=%q, want %q", got, want)
	}
}

func TestSystemChatUnmarshalNBTTranslateWithParams(t *testing.T) {
	body := append(nbtTranslateJoined("FeastGoBot"), 0x00)
	pkt := &PlayClientboundSystemChatMessagePacket{}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(body))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "multiplayer.player.joined FeastGoBot"
	if got := pkt.DisplayText(); got != want {
		t.Fatalf("DisplayText()=%q, want %q", got, want)
	}
}

func TestPlayerChatDisplayMessageWithSender(t *testing.T) {
	var body bytes.Buffer
	w := NewWriter(&body)
	_ = w.WriteUUID([16]byte{1, 2, 3})
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteString("hello from FeastGo full integration test")
	_ = w.WriteLong(123)
	_ = w.WriteLong(456)
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteVarInt(0)
	_ = w.WriteVarInt(1)
	_ = w.WriteString(`{"text":"FeastGoBot"}`)
	_ = w.WriteBoolean(false)

	pkt := &PlayClientboundPlayerChatMessagePacket{}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(body.Bytes()))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "FeastGoBot: hello from FeastGo full integration test"
	if got := pkt.DisplayMessage(); got != want {
		t.Fatalf("DisplayMessage()=%q, want %q", got, want)
	}
}

func TestPlayerChatDisplayMessageWithNBTSender(t *testing.T) {
	var body bytes.Buffer
	w := NewWriter(&body)
	_ = w.WriteUUID([16]byte{1, 2, 3})
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteString("hello from FeastGo full integration test")
	_ = w.WriteLong(123)
	_ = w.WriteLong(456)
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteVarInt(0)
	_ = w.WriteVarInt(1)
	body.Write(nbtStringRoot("FeastGoBot"))
	_ = w.WriteBoolean(false)

	pkt := &PlayClientboundPlayerChatMessagePacket{}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(body.Bytes()))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "FeastGoBot: hello from FeastGo full integration test"
	if got := pkt.DisplayMessage(); got != want {
		t.Fatalf("DisplayMessage()=%q, want %q", got, want)
	}
}

func TestPlayerChatDisplayMessageNoSender(t *testing.T) {
	var body bytes.Buffer
	w := NewWriter(&body)
	_ = w.WriteUUID([16]byte{1, 2, 3})
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteString("hello world")
	_ = w.WriteLong(123)
	_ = w.WriteLong(456)
	_ = w.WriteVarInt(0)
	_ = w.WriteBoolean(false)
	_ = w.WriteVarInt(0)
	_ = w.WriteVarInt(1)
	_ = w.WriteString(`{"text":"ab"}`) // sender name < 3 chars, should be omitted
	_ = w.WriteBoolean(false)

	pkt := &PlayClientboundPlayerChatMessagePacket{}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(body.Bytes()))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "hello world"
	if got := pkt.DisplayMessage(); got != want {
		t.Fatalf("DisplayMessage()=%q, want %q", got, want)
	}
}

func nbtTranslateJoined(name string) []byte {
	var out []byte
	out = append(out, 0x0a)
	out = appendNBTStringTag(out, "translate", "multiplayer.player.joined")
	out = append(out, 0x09)
	out = appendNBTString16(out, "with")
	out = append(out, 0x0a)
	out = append(out, 0x00, 0x00, 0x00, 0x01)
	out = appendNBTStringTag(out, "text", name)
	out = append(out, 0x00)
	out = append(out, 0x00)
	return out
}

func nbtStringRoot(value string) []byte {
	out := []byte{0x08}
	return appendNBTString16(out, value)
}

func appendNBTStringTag(out []byte, name, value string) []byte {
	out = append(out, 0x08)
	out = appendNBTString16(out, name)
	return appendNBTString16(out, value)
}

func appendNBTString16(out []byte, value string) []byte {
	out = append(out, byte(len(value)>>8), byte(len(value)))
	out = append(out, value...)
	return out
}
