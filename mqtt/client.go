package mqtt

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/baetyl/baetyl-go/log"
	"github.com/baetyl/baetyl-go/utils"
	"github.com/jpillora/backoff"
)

// Client auto reconnection client
type Client struct {
	cfg   ClientConfig
	obs   Observer
	tls   *tls.Config
	ids   *Counter
	cache chan Packet
	log   *log.Logger
	tomb  utils.Tomb
}

// NewClient creates a new client
func NewClient(cc ClientConfig, obs Observer) (*Client, error) {
	var err error
	var tc *tls.Config
	if cc.Certificate.Key != "" || cc.Certificate.Cert != "" {
		tc, err = utils.NewTLSConfigClient(cc.Certificate)
		if err != nil {
			return nil, err
		}
	}
	c := &Client{
		cfg:   cc,
		obs:   obs,
		tls:   tc,
		ids:   NewCounter(),
		cache: make(chan Packet, cc.BufferSize),
		log:   log.With(log.Any("mqtt", "client"), log.Any("cid", cc.ClientID)),
	}
	c.tomb.Go(c.connecting)
	return c, nil
}

// Subscribe sends a subscribe packet
func (c *Client) Subscribe(s []Subscription) error {
	subscribe := &Subscribe{
		ID:            c.ids.NextID(),
		Subscriptions: s,
	}
	return c.Send(subscribe)
}

// Publish sends a publish packet
func (c *Client) Publish(qos QOS, topic string, payload []byte, pid ID, retain bool, dup bool) error {
	publish := NewPublish()
	publish.ID = pid
	publish.Dup = dup
	publish.Message.QOS = qos
	publish.Message.Topic = topic
	publish.Message.Payload = payload
	publish.Message.Retain = retain
	if qos != 0 && pid == 0 {
		publish.ID = c.ids.NextID()
	}
	return c.Send(publish)
}

// Send sends a generic packet
func (c *Client) Send(pkt Packet) error {
	select {
	case c.cache <- pkt:
		return nil
	case <-c.tomb.Dying():
		return ErrClientAlreadyClosed
	}
}

// Close closes client
func (c *Client) Close() error {
	c.log.Info("client is closing")
	defer c.log.Info("client has closed")

	c.tomb.Kill(nil)
	return c.tomb.Wait()
}

func (c *Client) connecting() error {
	c.log.Info("client starts to keep connecting")
	defer c.log.Info("client has stopped connecting")

	var err error
	var curr Packet
	var stream *stream
	var next time.Time
	timer := time.NewTimer(0)
	defer timer.Stop()
	bf := backoff.Backoff{
		Min:    time.Second,
		Max:    c.cfg.Interval,
		Factor: 1.6,
	}

	for {
		if !next.IsZero() {
			timer.Reset(next.Sub(time.Now()))
			c.log.Info("next reconnect", log.Any("at", next), log.Any("attempt", bf.Attempt()))
		}
		if stream != nil {
			stream.close()
			stream = nil
			c.log.Info("client has disconnected")
		}
		select {
		case <-c.tomb.Dying():
			return nil
		case <-timer.C:
		}

		c.log.Info("client starts to connect")
		next = time.Now().Add(bf.Duration())
		stream, err = c.connect()
		if err != nil {
			c.onError("failed to connect", err)
			continue
		}
		c.log.Info("client has connected")
		bf.Reset()
		curr = stream.sending(curr)
	}
}

func (c *Client) onConnack(pkt Packet) error {
	p, ok := pkt.(*Connack)
	if !ok {
		return ErrClientExpectedConnack
	}
	if p.ReturnCode != ConnectionAccepted {
		return fmt.Errorf(p.ReturnCode.String())
	}
	return nil
}

func (c *Client) onPublish(pkt *Publish) error {
	if c.obs == nil {
		return nil
	}
	return c.obs.OnPublish(pkt)
}

func (c *Client) onPuback(pkt *Puback) error {
	if c.obs == nil {
		return nil
	}
	return c.obs.OnPuback(pkt)
}

func (c *Client) onSuback(pkt *Suback) error {
	for _, code := range pkt.ReturnCodes {
		if code == QOSFailure {
			return ErrClientSubscriptionFailed
		}
	}
	return nil
}

func (c *Client) onError(msg string, err error) {
	if c.obs == nil || err == nil {
		return
	}
	c.log.Error(msg, log.Error(err))
	c.obs.OnError(err)
}
