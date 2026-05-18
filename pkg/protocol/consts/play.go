package consts

// Play packet IDs for Minecraft 1.20.4 (protocol 765).
const (
	// Bundle Delimiter (clientbound)
	PlayClientboundBundleDelimiter int32 = 0x00
	// Spawn Entity (clientbound)
	PlayClientboundSpawnEntity int32 = 0x01
	// Spawn Experience Orb (clientbound)
	PlayClientboundSpawnExperienceOrb int32 = 0x02
	// Entity Animation (clientbound)
	PlayClientboundEntityAnimation int32 = 0x03
	// Award Statistics (clientbound)
	PlayClientboundAwardStatistics int32 = 0x04
	// Acknowledge Block Change (clientbound)
	PlayClientboundAcknowledgeBlockChange int32 = 0x05
	// Set Block Destroy Stage (clientbound)
	PlayClientboundSetBlockDestroyStage int32 = 0x06
	// Block Entity Data (clientbound)
	PlayClientboundBlockEntityData int32 = 0x07
	// Block Action (clientbound)
	PlayClientboundBlockAction int32 = 0x08
	// Block Update (clientbound)
	PlayClientboundBlockUpdate int32 = 0x09
	// Boss Bar (clientbound)
	PlayClientboundBossBar int32 = 0x0A
	// Change Difficulty (clientbound)
	PlayClientboundChangeDifficulty int32 = 0x0B
	// Chunk Batch Finished (clientbound)
	PlayClientboundChunkBatchFinished int32 = 0x0C
	// Chunk Batch Start (clientbound)
	PlayClientboundChunkBatchStart int32 = 0x0D
	// Chunk Biomes (clientbound)
	PlayClientboundChunkBiomes int32 = 0x0E
	// Clear Titles (clientbound)
	PlayClientboundClearTitles int32 = 0x0F
	// Command Suggestions Response (clientbound)
	PlayClientboundCommandSuggestionsResponse int32 = 0x10
	// Commands (clientbound)
	PlayClientboundCommands int32 = 0x11
	// Close Container (clientbound)
	PlayClientboundCloseContainer int32 = 0x12
	// Set Container Content (clientbound)
	PlayClientboundSetContainerContent int32 = 0x13
	// Set Container Property (clientbound)
	PlayClientboundSetContainerProperty int32 = 0x14
	// Set Container Slot (clientbound)
	PlayClientboundSetContainerSlot int32 = 0x15
	// Set Cooldown (clientbound)
	PlayClientboundSetCooldown int32 = 0x16
	// Chat Suggestions (clientbound)
	PlayClientboundChatSuggestions int32 = 0x17
	// Clientbound Plugin Message (play) (clientbound)
	PlayClientboundClientboundPluginMessage int32 = 0x18
	// Damage Event (clientbound)
	PlayClientboundDamageEvent int32 = 0x19
	// Delete Message (clientbound)
	PlayClientboundDeleteMessage int32 = 0x1A
	// Disconnect (play) (clientbound)
	PlayClientboundDisconnect int32 = 0x1B
	// Disguised Chat Message (clientbound)
	PlayClientboundDisguisedChatMessage int32 = 0x1C
	// Entity Event (clientbound)
	PlayClientboundEntityEvent int32 = 0x1D
	// Explosion (clientbound)
	PlayClientboundExplosion int32 = 0x1E
	// Unload Chunk (clientbound)
	PlayClientboundUnloadChunk int32 = 0x1F
	// Game Event (clientbound)
	PlayClientboundGameEvent int32 = 0x20
	// Open Horse Screen (clientbound)
	PlayClientboundOpenHorseScreen int32 = 0x21
	// Hurt Animation (clientbound)
	PlayClientboundHurtAnimation int32 = 0x22
	// Initialize World Border (clientbound)
	PlayClientboundInitializeWorldBorder int32 = 0x23
	// Clientbound Keep Alive (play) (clientbound)
	PlayClientboundClientboundKeepAlive int32 = 0x24
	// Chunk Data and Update Light (clientbound)
	PlayClientboundChunkDataAndUpdateLight int32 = 0x25
	// World Event (clientbound)
	PlayClientboundWorldEvent int32 = 0x26
	// Particle (clientbound)
	PlayClientboundParticle int32 = 0x27
	// Update Light (clientbound)
	PlayClientboundUpdateLight int32 = 0x28
	// Login (play) (clientbound)
	PlayClientboundLogin int32 = 0x29
	// Map Data (clientbound)
	PlayClientboundMapData int32 = 0x2A
	// Merchant Offers (clientbound)
	PlayClientboundMerchantOffers int32 = 0x2B
	// Update Entity Position (clientbound)
	PlayClientboundUpdateEntityPosition int32 = 0x2C
	// Update Entity Position and Rotation (clientbound)
	PlayClientboundUpdateEntityPositionAndRotation int32 = 0x2D
	// Update Entity Rotation (clientbound)
	PlayClientboundUpdateEntityRotation int32 = 0x2E
	// Move Vehicle (clientbound)
	PlayClientboundMoveVehicle int32 = 0x2F
	// Open Book (clientbound)
	PlayClientboundOpenBook int32 = 0x30
	// Open Screen (clientbound)
	PlayClientboundOpenScreen int32 = 0x31
	// Open Sign Editor (clientbound)
	PlayClientboundOpenSignEditor int32 = 0x32
	// Ping (play) (clientbound)
	PlayClientboundPing int32 = 0x33
	// Ping Response (play) (clientbound)
	PlayClientboundPingResponse int32 = 0x34
	// Place Ghost Recipe (clientbound)
	PlayClientboundPlaceGhostRecipe int32 = 0x35
	// Player Abilities (clientbound) (clientbound)
	PlayClientboundPlayerAbilities int32 = 0x36
	// Player Chat Message (clientbound)
	PlayClientboundPlayerChatMessage int32 = 0x37
	// End Combat (clientbound)
	PlayClientboundEndCombat int32 = 0x38
	// Enter Combat (clientbound)
	PlayClientboundEnterCombat int32 = 0x39
	// Combat Death (clientbound)
	PlayClientboundCombatDeath int32 = 0x3A
	// Player Info Remove (clientbound)
	PlayClientboundPlayerInfoRemove int32 = 0x3B
	// Player Info Update (clientbound)
	PlayClientboundPlayerInfoUpdate int32 = 0x3C
	// Look At (clientbound)
	PlayClientboundLookAt int32 = 0x3D
	// Synchronize Player Position (clientbound)
	PlayClientboundSynchronizePlayerPosition int32 = 0x3E
	// Update Recipe Book (clientbound)
	PlayClientboundUpdateRecipeBook int32 = 0x3F
	// Remove Entities (clientbound)
	PlayClientboundRemoveEntities int32 = 0x40
	// Remove Entity Effect (clientbound)
	PlayClientboundRemoveEntityEffect int32 = 0x41
	// Reset Score (clientbound)
	PlayClientboundResetScore int32 = 0x42
	// Remove Resource Pack (play) (clientbound)
	PlayClientboundRemoveResourcePack int32 = 0x43
	// Add Resource Pack (play) (clientbound)
	PlayClientboundAddResourcePack int32 = 0x44
	// Respawn (clientbound)
	PlayClientboundRespawn int32 = 0x45
	// Set Head Rotation (clientbound)
	PlayClientboundSetHeadRotation int32 = 0x46
	// Update Section Blocks (clientbound)
	PlayClientboundUpdateSectionBlocks int32 = 0x47
	// Select Advancements Tab (clientbound)
	PlayClientboundSelectAdvancementsTab int32 = 0x48
	// Server Data (clientbound)
	PlayClientboundServerData int32 = 0x49
	// Set Action Bar Text (clientbound)
	PlayClientboundSetActionBarText int32 = 0x4A
	// Set Border Center (clientbound)
	PlayClientboundSetBorderCenter int32 = 0x4B
	// Set Border Lerp Size (clientbound)
	PlayClientboundSetBorderLerpSize int32 = 0x4C
	// Set Border Size (clientbound)
	PlayClientboundSetBorderSize int32 = 0x4D
	// Set Border Warning Delay (clientbound)
	PlayClientboundSetBorderWarningDelay int32 = 0x4E
	// Set Border Warning Distance (clientbound)
	PlayClientboundSetBorderWarningDistance int32 = 0x4F
	// Set Camera (clientbound)
	PlayClientboundSetCamera int32 = 0x50
	// Set Held Item (clientbound) (clientbound)
	PlayClientboundSetHeldItem int32 = 0x51
	// Set Center Chunk (clientbound)
	PlayClientboundSetCenterChunk int32 = 0x52
	// Set Render Distance (clientbound)
	PlayClientboundSetRenderDistance int32 = 0x53
	// Set Default Spawn Position (clientbound)
	PlayClientboundSetDefaultSpawnPosition int32 = 0x54
	// Display Objective (clientbound)
	PlayClientboundDisplayObjective int32 = 0x55
	// Set Entity Metadata (clientbound)
	PlayClientboundSetEntityMetadata int32 = 0x56
	// Link Entities (clientbound)
	PlayClientboundLinkEntities int32 = 0x57
	// Set Entity Velocity (clientbound)
	PlayClientboundSetEntityVelocity int32 = 0x58
	// Set Equipment (clientbound)
	PlayClientboundSetEquipment int32 = 0x59
	// Set Experience (clientbound)
	PlayClientboundSetExperience int32 = 0x5A
	// Set Health (clientbound)
	PlayClientboundSetHealth int32 = 0x5B
	// Update Objectives (clientbound)
	PlayClientboundUpdateObjectives int32 = 0x5C
	// Set Passengers (clientbound)
	PlayClientboundSetPassengers int32 = 0x5D
	// Update Teams (clientbound)
	PlayClientboundUpdateTeams int32 = 0x5E
	// Update Score (clientbound)
	PlayClientboundUpdateScore int32 = 0x5F
	// Set Simulation Distance (clientbound)
	PlayClientboundSetSimulationDistance int32 = 0x60
	// Set Subtitle Text (clientbound)
	PlayClientboundSetSubtitleText int32 = 0x61
	// Update Time (clientbound)
	PlayClientboundUpdateTime int32 = 0x62
	// Set Title Text (clientbound)
	PlayClientboundSetTitleText int32 = 0x63
	// Set Title Animation Times (clientbound)
	PlayClientboundSetTitleAnimationTimes int32 = 0x64
	// Entity Sound Effect (clientbound)
	PlayClientboundEntitySoundEffect int32 = 0x65
	// Sound Effect (clientbound)
	PlayClientboundSoundEffect int32 = 0x66
	// Start Configuration (clientbound)
	PlayClientboundStartConfiguration int32 = 0x67
	// Stop Sound (clientbound)
	PlayClientboundStopSound int32 = 0x68
	// System Chat Message (clientbound)
	PlayClientboundSystemChatMessage int32 = 0x69
	// Set Tab List Header And Footer (clientbound)
	PlayClientboundSetTabListHeaderAndFooter int32 = 0x6A
	// Tag Query Response (clientbound)
	PlayClientboundTagQueryResponse int32 = 0x6B
	// Pickup Item (clientbound)
	PlayClientboundPickupItem int32 = 0x6C
	// Teleport Entity (clientbound)
	PlayClientboundTeleportEntity int32 = 0x6D
	// Set Ticking State (clientbound)
	PlayClientboundSetTickingState int32 = 0x6E
	// Step Tick (clientbound)
	PlayClientboundStepTick int32 = 0x6F
	// Update Advancements (clientbound)
	PlayClientboundUpdateAdvancements int32 = 0x70
	// Update Attributes (clientbound)
	PlayClientboundUpdateAttributes int32 = 0x71
	// Entity Effect (clientbound)
	PlayClientboundEntityEffect int32 = 0x72
	// Update Recipes (clientbound)
	PlayClientboundUpdateRecipes int32 = 0x73
	// Update Tags (play) (clientbound)
	PlayClientboundUpdateTags int32 = 0x74
	// Confirm Teleportation (serverbound)
	PlayServerboundConfirmTeleportation int32 = 0x00
	// Query Block Entity Tag (serverbound)
	PlayServerboundQueryBlockEntityTag int32 = 0x01
	// Change Difficulty (serverbound)
	PlayServerboundChangeDifficulty int32 = 0x02
	// Acknowledge Message (serverbound)
	PlayServerboundAcknowledgeMessage int32 = 0x03
	// Chat Command (serverbound)
	PlayServerboundChatCommand int32 = 0x04
	// Chat Message (serverbound)
	PlayServerboundChatMessage int32 = 0x05
	// Player Session (serverbound)
	PlayServerboundPlayerSession int32 = 0x06
	// Chunk Batch Received (serverbound)
	PlayServerboundChunkBatchReceived int32 = 0x07
	// Client Status (serverbound)
	PlayServerboundClientStatus int32 = 0x08
	// Client Information (play) (serverbound)
	PlayServerboundClientInformation int32 = 0x09
	// Command Suggestions Request (serverbound)
	PlayServerboundCommandSuggestionsRequest int32 = 0x0A
	// Acknowledge Configuration (serverbound)
	PlayServerboundAcknowledgeConfiguration int32 = 0x0B
	// Click Container Button (serverbound)
	PlayServerboundClickContainerButton int32 = 0x0C
	// Click Container (serverbound)
	PlayServerboundClickContainer int32 = 0x0D
	// Close Container (serverbound)
	PlayServerboundCloseContainer int32 = 0x0E
	// Change Container Slot State (serverbound)
	PlayServerboundChangeContainerSlotState int32 = 0x0F
	// Serverbound Plugin Message (play) (serverbound)
	PlayServerboundServerboundPluginMessage int32 = 0x10
	// Edit Book (serverbound)
	PlayServerboundEditBook int32 = 0x11
	// Query Entity Tag (serverbound)
	PlayServerboundQueryEntityTag int32 = 0x12
	// Interact (serverbound)
	PlayServerboundInteract int32 = 0x13
	// Jigsaw Generate (serverbound)
	PlayServerboundJigsawGenerate int32 = 0x14
	// Serverbound Keep Alive (play) (serverbound)
	PlayServerboundServerboundKeepAlive int32 = 0x15
	// Lock Difficulty (serverbound)
	PlayServerboundLockDifficulty int32 = 0x16
	// Set Player Position (serverbound)
	PlayServerboundSetPlayerPosition int32 = 0x17
	// Set Player Position and Rotation (serverbound)
	PlayServerboundSetPlayerPositionAndRotation int32 = 0x18
	// Set Player Rotation (serverbound)
	PlayServerboundSetPlayerRotation int32 = 0x19
	// Set Player On Ground (serverbound)
	PlayServerboundSetPlayerOnGround int32 = 0x1A
	// Move Vehicle (serverbound)
	PlayServerboundMoveVehicle int32 = 0x1B
	// Paddle Boat (serverbound)
	PlayServerboundPaddleBoat int32 = 0x1C
	// Pick Item (serverbound)
	PlayServerboundPickItem int32 = 0x1D
	// Ping Request (play) (serverbound)
	PlayServerboundPingRequest int32 = 0x1E
	// Place Recipe (serverbound)
	PlayServerboundPlaceRecipe int32 = 0x1F
	// Player Abilities (serverbound) (serverbound)
	PlayServerboundPlayerAbilities int32 = 0x20
	// Player Action (serverbound)
	PlayServerboundPlayerAction int32 = 0x21
	// Player Command (serverbound)
	PlayServerboundPlayerCommand int32 = 0x22
	// Player Input (serverbound)
	PlayServerboundPlayerInput int32 = 0x23
	// Pong (play) (serverbound)
	PlayServerboundPong int32 = 0x24
	// Change Recipe Book Settings (serverbound)
	PlayServerboundChangeRecipeBookSettings int32 = 0x25
	// Set Seen Recipe (serverbound)
	PlayServerboundSetSeenRecipe int32 = 0x26
	// Rename Item (serverbound)
	PlayServerboundRenameItem int32 = 0x27
	// Resource Pack Response (play) (serverbound)
	PlayServerboundResourcePackResponse int32 = 0x28
	// Seen Advancements (serverbound)
	PlayServerboundSeenAdvancements int32 = 0x29
	// Select Trade (serverbound)
	PlayServerboundSelectTrade int32 = 0x2A
	// Set Beacon Effect (serverbound)
	PlayServerboundSetBeaconEffect int32 = 0x2B
	// Set Held Item (serverbound) (serverbound)
	PlayServerboundSetHeldItem int32 = 0x2C
	// Program Command Block (serverbound)
	PlayServerboundProgramCommandBlock int32 = 0x2D
	// Program Command Block Minecart (serverbound)
	PlayServerboundProgramCommandBlockMinecart int32 = 0x2E
	// Set Creative Mode Slot (serverbound)
	PlayServerboundSetCreativeModeSlot int32 = 0x2F
	// Program Jigsaw Block (serverbound)
	PlayServerboundProgramJigsawBlock int32 = 0x30
	// Program Structure Block (serverbound)
	PlayServerboundProgramStructureBlock int32 = 0x31
	// Update Sign (serverbound)
	PlayServerboundUpdateSign int32 = 0x32
	// Swing Arm (serverbound)
	PlayServerboundSwingArm int32 = 0x33
	// Teleport To Entity (serverbound)
	PlayServerboundTeleportToEntity int32 = 0x34
	// Use Item On (serverbound)
	PlayServerboundUseItemOn int32 = 0x35
	// Use Item (serverbound)
	PlayServerboundUseItem int32 = 0x36
)
