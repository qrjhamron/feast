package feast

import (
	"bytes"
	"crypto/md5"
	"fmt"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
)

func (c *Client) runLoginFlow() error {
	if err := c.requireConn(); err != nil {
		return err
	}

	if err := c.writePacket(&protocol.LoginServerboundStartPacket{
		Username: c.opts.Username,
		UUID:     offlineUUID(c.opts.Username),
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
		case consts.LoginClientboundDisconnect:
			pkt := &protocol.LoginClientboundDisconnectPacket{}
			if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
				return err
			}
			return fmt.Errorf("login disconnect: %s", pkt.Reason)
		case consts.LoginClientboundSetCompression:
			pkt := &protocol.LoginClientboundSetCompressionPacket{}
			if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
				return err
			}
			c.conn.SetCompression(int(pkt.Threshold))
		case consts.LoginClientboundLoginSuccess:
			if err := c.writePacket(&protocol.LoginServerboundAcknowledgedPacket{}); err != nil {
				return err
			}
			return nil
		case consts.LoginClientboundEncryptionRequest:
			return fmt.Errorf("online-mode encryption is intentionally unsupported")
		case consts.LoginClientboundLoginPluginRequest:
			// NOTE: Plugin requests are not handled in the current version.
			continue
		default:
			continue
		}
	}
}

func offlineUUID(username string) [16]byte {
	sum := md5.Sum([]byte("OfflinePlayer:" + username))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return sum
}
