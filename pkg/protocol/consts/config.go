package consts

// Configuration packet IDs for Minecraft 1.20.4 (protocol 765).
const (
	// Clientbound Plugin Message (configuration) (clientbound)
	ConfigurationClientboundClientboundPluginMessage int32 = 0x00
	// Disconnect (configuration) (clientbound)
	ConfigurationClientboundDisconnect int32 = 0x01
	// Finish Configuration (clientbound)
	ConfigurationClientboundFinishConfiguration int32 = 0x02
	// Clientbound Keep Alive (configuration) (clientbound)
	ConfigurationClientboundClientboundKeepAlive int32 = 0x03
	// Ping (configuration) (clientbound)
	ConfigurationClientboundPing int32 = 0x04
	// Registry Data (clientbound)
	ConfigurationClientboundRegistryData int32 = 0x05
	// Remove Resource Pack (configuration) (clientbound)
	ConfigurationClientboundRemoveResourcePack int32 = 0x06
	// Add Resource Pack (configuration) (clientbound)
	ConfigurationClientboundAddResourcePack int32 = 0x07
	// Feature Flags (clientbound)
	ConfigurationClientboundFeatureFlags int32 = 0x08
	// Update Tags (configuration) (clientbound)
	ConfigurationClientboundUpdateTags int32 = 0x09
	// Client Information (configuration) (serverbound)
	ConfigurationServerboundClientInformation int32 = 0x00
	// Serverbound Plugin Message (configuration) (serverbound)
	ConfigurationServerboundServerboundPluginMessage int32 = 0x01
	// Acknowledge Finish Configuration (serverbound)
	ConfigurationServerboundAcknowledgeFinishConfiguration int32 = 0x02
	// Serverbound Keep Alive (configuration) (serverbound)
	ConfigurationServerboundServerboundKeepAlive int32 = 0x03
	// Pong (configuration) (serverbound)
	ConfigurationServerboundPong int32 = 0x04
	// Resource Pack Response (configuration) (serverbound)
	ConfigurationServerboundResourcePackResponse int32 = 0x05
)
