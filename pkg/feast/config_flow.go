package feast

import (
	"bytes"
	"fmt"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

func (c *Client) runConfigFlow() error {
	if err := c.requireConn(); err != nil {
		return err
	}

	// Send Client Information as required by Configuration state
	if err := c.writePacket(&protocol.ConfigServerboundClientInformationPacket{
		Locale:              "en_us",
		ViewDistance:        10,
		ChatMode:            0,
		ChatColors:          true,
		DisplayedSkinParts:  0x7f,
		MainHand:            1,
		EnableTextFiltering: false,
		AllowServerListings: true,
	}); err != nil {
		return err
	}

	for {
		raw, err := c.readPacket()
		if err != nil {
			return err
		}

		if err := c.dispatcher.Dispatch(c.currentState(), raw); err != nil {
			return err
		}

		switch raw.ID {
		case consts.ConfigurationClientboundDisconnect:
			pkt := &protocol.ConfigClientboundDisconnectPacket{}
			if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
				return err
			}
			return fmt.Errorf("config disconnect: %s", pkt.Reason)
		case consts.ConfigurationClientboundClientboundKeepAlive:
			pkt := &protocol.ConfigClientboundKeepAlivePacket{}
			if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
				return err
			}
			if err := c.writePacket(&protocol.ConfigServerboundKeepAlivePacket{KeepAliveID: pkt.KeepAliveID}); err != nil {
				return err
			}
		case consts.ConfigurationClientboundPing:
			pkt := &protocol.ConfigClientboundPingPacket{}
			if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
				return err
			}
			if err := c.writePacket(&protocol.ConfigServerboundPongPacket{ID: pkt.ID}); err != nil {
				return err
			}
		case consts.ConfigurationClientboundFinishConfiguration:
			if err := c.writePacket(&protocol.ConfigServerboundAcknowledgeFinishPacket{}); err != nil {
				return err
			}
			return nil
		default:
			continue
		}
	}
}
