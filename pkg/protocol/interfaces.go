package protocol

// Packet represents a typed Minecraft packet.
type Packet interface {
	// PacketID returns the packet ID for the current protocol state/direction.
	PacketID() int32
	// Marshal encodes the packet body into the writer.
	Marshal(w *Writer) error
	// Unmarshal decodes the packet body from the reader.
	Unmarshal(r *Reader) error
}
