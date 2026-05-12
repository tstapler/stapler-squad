package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/tstapler/stapler-squad/log"
)

type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type PushNotification struct {
	Title              string                 `json:"title"`
	Body               string                 `json:"body"`
	Icon               string                 `json:"icon,omitempty"`
	Tag                string                 `json:"tag,omitempty"`
	Data               map[string]interface{} `json:"data,omitempty"`
	RequireInteraction bool                   `json:"requireInteraction,omitempty"`
	Renotify           bool                   `json:"renotify,omitempty"`
}

type PushService struct {
	subscriptions   map[string]PushSubscription
	vapidPrivateKey *ecdsa.PrivateKey
	vapidPublicKey  string
	mu              sync.RWMutex
	subsPath        string
	vapidKeyPath    string
}

func NewPushService(configDir string) *PushService {
	ps := &PushService{
		subscriptions: make(map[string]PushSubscription),
		subsPath:      filepath.Join(configDir, "push-subscriptions.json"),
		vapidKeyPath:  filepath.Join(configDir, "vapid-keys.json"),
	}

	if err := ps.loadVapidKeys(); err != nil {
		log.Error("failed to load VAPID keys", "err", err)
		if err := ps.generateVapidKeys(); err != nil {
			log.Error("failed to generate VAPID keys", "err", err)
		}
	}

	if err := ps.loadSubscriptions(); err != nil {
		log.Error("failed to load push subscriptions", "err", err)
	}

	return ps
}

func (ps *PushService) generateVapidKeys() error {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate VAPID private key: %w", err)
	}

	ps.vapidPrivateKey = privateKey

	publicKey := &privateKey.PublicKey
	publicKeyBytes := elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y) //nolint:staticcheck
	ps.vapidPublicKey = base64.RawURLEncoding.EncodeToString(publicKeyBytes)

	vapidData := map[string]string{
		"privateKey": base64.RawURLEncoding.EncodeToString(privateKey.D.Bytes()),
		"publicKey":  ps.vapidPublicKey,
	}

	data, err := json.Marshal(vapidData)
	if err != nil {
		return fmt.Errorf("failed to marshal VAPID keys: %w", err)
	}

	if err := os.WriteFile(ps.vapidKeyPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write VAPID keys: %w", err)
	}

	log.Info("generated new VAPID keys")
	return nil
}

func (ps *PushService) loadVapidKeys() error {
	data, err := os.ReadFile(ps.vapidKeyPath)
	if err != nil {
		return err
	}

	var vapidData map[string]string
	if err := json.Unmarshal(data, &vapidData); err != nil {
		return err
	}

	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(vapidData["privateKey"])
	if err != nil {
		return err
	}

	publicKeyBytes, err := base64.RawURLEncoding.DecodeString(vapidData["publicKey"])
	if err != nil {
		return err
	}

	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
		D: new(big.Int),
	}

	privateKey.D.SetBytes(privateKeyBytes)
	privateKey.PublicKey.X, privateKey.PublicKey.Y = elliptic.Unmarshal(elliptic.P256(), publicKeyBytes) //nolint:staticcheck
	if privateKey.PublicKey.X == nil {                                                                   //nolint:staticcheck
		return fmt.Errorf("invalid public key")
	}

	ps.vapidPrivateKey = privateKey
	ps.vapidPublicKey = vapidData["publicKey"]

	return nil
}

func (ps *PushService) GetVapidPublicKey() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.vapidPublicKey
}

func (ps *PushService) Subscribe(sub PushSubscription) string {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	hash := sha256.Sum256([]byte(sub.Endpoint))
	subID := base64.RawURLEncoding.EncodeToString(hash[:])

	ps.subscriptions[subID] = sub
	ps.saveSubscriptions()

	log.Info("added push subscription", "sub_id_prefix", subID[:8])
	return subID
}

func (ps *PushService) Unsubscribe(endpoint string) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	hash := sha256.Sum256([]byte(endpoint))
	subID := base64.RawURLEncoding.EncodeToString(hash[:])

	if _, ok := ps.subscriptions[subID]; ok {
		delete(ps.subscriptions, subID)
		ps.saveSubscriptions()
		log.Info("removed push subscription", "sub_id_prefix", subID[:8])
		return true
	}

	return false
}

func (ps *PushService) GetSubscriptions() []PushSubscription {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	subs := make([]PushSubscription, 0, len(ps.subscriptions))
	for _, sub := range ps.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

func (ps *PushService) SendNotification(notif PushNotification) int {
	ps.mu.RLock()
	subs := make([]PushSubscription, 0, len(ps.subscriptions))
	for _, sub := range ps.subscriptions {
		subs = append(subs, sub)
	}
	ps.mu.RUnlock()

	if len(subs) == 0 {
		return 0
	}

	successCount := 0
	for _, sub := range subs {
		if err := ps.sendToSubscription(sub, notif); err != nil {
			endpoint := sub.Endpoint
			if len(endpoint) > 50 {
				endpoint = endpoint[:50]
			}
			log.Error("failed to send push notification", "endpoint_prefix", endpoint, "err", err)
			continue
		}
		successCount++
	}

	return successCount
}

func (ps *PushService) sendToSubscription(sub PushSubscription, notif PushNotification) error {
	if ps.vapidPrivateKey == nil {
		return fmt.Errorf("VAPID private key not initialized")
	}

	// Only works for Chrome/Firefox push (not Safari)
	// Safari uses APNs which requires additional setup
	if sub.Endpoint == "" {
		return fmt.Errorf("empty endpoint")
	}

	body, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	subInfo := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.Keys.P256dh,
			Auth:   sub.Keys.Auth,
		},
	}

	// Convert private key to PEM format for webpush-go library
	privateKeyPEM, err := ps.getPrivateKeyPEM()
	if err != nil {
		return fmt.Errorf("failed to get private key PEM: %w", err)
	}

	resp, err := webpush.SendNotification(body, subInfo, &webpush.Options{
		VAPIDPrivateKey: privateKeyPEM,
		VAPIDPublicKey:  ps.vapidPublicKey,
		TTL:             60 * 60 * 24, // 24 hours
	})
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode == 410 || resp.StatusCode == 404 {
			ps.Unsubscribe(sub.Endpoint)
			return nil
		}
	}
	return err
}

// getPrivateKeyPEM returns the VAPID private key as a base64url-encoded string
// (raw 32-byte D value), which is the format expected by webpush-go.
func (ps *PushService) getPrivateKeyPEM() (string, error) {
	if ps.vapidPrivateKey == nil {
		return "", fmt.Errorf("VAPID private key not initialized")
	}
	return base64.RawURLEncoding.EncodeToString(ps.vapidPrivateKey.D.Bytes()), nil
}

func (ps *PushService) loadSubscriptions() error {
	data, err := os.ReadFile(ps.subsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var subs map[string]PushSubscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return err
	}

	ps.subscriptions = subs
	return nil
}

func (ps *PushService) saveSubscriptions() {
	data, err := json.Marshal(ps.subscriptions)
	if err != nil {
		log.Error("failed to marshal subscriptions", "err", err)
		return
	}

	if err := os.WriteFile(ps.subsPath, data, 0600); err != nil {
		log.Error("failed to save subscriptions", "err", err)
	}
}
