package consts

// Login packet IDs for Minecraft 1.20.4 (protocol 765).
const (
	// Disconnect (login) (clientbound)
	LoginClientboundDisconnect int32 = 0x00
	// Encryption Request (clientbound)
	LoginClientboundEncryptionRequest int32 = 0x01
	// Login Success (clientbound)
	LoginClientboundLoginSuccess int32 = 0x02
	// Set Compression (clientbound)
	LoginClientboundSetCompression int32 = 0x03
	// Login Plugin Request (clientbound)
	LoginClientboundLoginPluginRequest int32 = 0x04
	// Login Start (serverbound)
	LoginServerboundLoginStart int32 = 0x00
	// Encryption Response (serverbound)
	LoginServerboundEncryptionResponse int32 = 0x01
	// Login Plugin Response (serverbound)
	LoginServerboundLoginPluginResponse int32 = 0x02
	// Login Acknowledged (serverbound)
	LoginServerboundLoginAcknowledged int32 = 0x03
)
