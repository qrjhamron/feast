package protocol

import (
	"bytes"
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
		{"sb_client_info", &ConfigServerboundClientInformationPacket{Data: []byte{0x02, 0x01}}, func() Packet { return &ConfigServerboundClientInformationPacket{} }},
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
		{"sb_confirm_teleport", &PlayServerboundConfirmTeleportationPacket{TeleportID: 77}, func() Packet { return &PlayServerboundConfirmTeleportationPacket{} }},
		{"sb_chat_message", &PlayServerboundChatMessagePacket{Message: "hello", Timestamp: 1, Salt: 2, MessageCount: 3}, func() Packet { return &PlayServerboundChatMessagePacket{} }},
		{"sb_set_pos_rot", &PlayServerboundSetPlayerPositionAndRotationPacket{X: 1, Y: 2, Z: 3, Yaw: 4, Pitch: 5, OnGround: true}, func() Packet { return &PlayServerboundSetPlayerPositionAndRotationPacket{} }},
		{"sb_player_action", &PlayServerboundPlayerActionPacket{RawData: []byte{0x01, 0x02}}, func() Packet { return &PlayServerboundPlayerActionPacket{} }},
		{"sb_use_item_on", &PlayServerboundUseItemOnPacket{RawData: []byte{0x03, 0x04}}, func() Packet { return &PlayServerboundUseItemOnPacket{} }},
		{"sb_keepalive", &PlayServerboundKeepAlivePacket{KeepAliveID: 98765}, func() Packet { return &PlayServerboundKeepAlivePacket{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripPacket(t, tc.pkt, tc.clone)
		})
	}
}
