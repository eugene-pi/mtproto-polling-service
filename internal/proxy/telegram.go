package proxy

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
)

const defaultVerifyTimeout = 15 * time.Second

// ErrMissingCredentials is returned when a TelegramVerifier is built without
// Telegram api_id/api_hash.
var ErrMissingCredentials = errors.New("telegram api id and api hash are required")

// TelegramVerifier performs the second-stage check: it confirms that a real
// Telegram client can actually use a proxy. It connects to a Telegram data
// center *through* the MTProxy, completes the MTProto handshake and issues one
// unauthenticated RPC (help.getConfig). A nil error means a Telegram client
// could sign in through this proxy.
//
// It uses gotd/td with an in-memory session, so no account, phone number or
// login code is involved — only the api_id/api_hash, which are required.
type TelegramVerifier struct {
	apiID   int
	apiHash string
	timeout time.Duration
}

// NewTelegramVerifier builds a verifier. apiID and apiHash are required and a
// missing value returns ErrMissingCredentials.
func NewTelegramVerifier(apiID int, apiHash string, timeout time.Duration) (*TelegramVerifier, error) {
	if apiID == 0 || apiHash == "" {
		return nil, ErrMissingCredentials
	}
	if timeout <= 0 {
		timeout = defaultVerifyTimeout
	}
	return &TelegramVerifier{apiID: apiID, apiHash: apiHash, timeout: timeout}, nil
}

// Verify reports whether a Telegram client can use the proxy. It returns nil on
// success and a descriptive error otherwise.
func (v *TelegramVerifier) Verify(ctx context.Context, p Proxy) error {
	secret, err := hex.DecodeString(p.Secret)
	if err != nil {
		return fmt.Errorf("decode proxy secret: %w", err)
	}

	resolver, err := dcs.MTProxy(p.Address(), secret, dcs.MTProxyOptions{})
	if err != nil {
		return fmt.Errorf("build mtproxy resolver: %w", err)
	}

	client := telegram.NewClient(v.apiID, v.apiHash, telegram.Options{
		Resolver:  resolver,
		NoUpdates: true,
	})

	runCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	return client.Run(runCtx, func(ctx context.Context) error {
		// An unauthenticated round-trip through the proxy to Telegram. It also
		// wraps initConnection, so it validates the api_id/api_hash too.
		if _, err := client.API().HelpGetConfig(ctx); err != nil {
			return fmt.Errorf("help.getConfig: %w", err)
		}
		return nil
	})
}
