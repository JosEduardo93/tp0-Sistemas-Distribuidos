package common

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
	Bet           Bet
}

// Struct for the bet
type Bet struct {
	Id         int
	Nombre     string
	Apellido   string
	Documento  int
	Nacimiento string
	Numero     int
}

func (b *Bet) serialize() []byte {
	return []byte(fmt.Sprintf(
		"%d;%s;%s;%d;%s;%d",
		b.Id,
		b.Nombre,
		b.Apellido,
		b.Documento,
		b.Nacimiento,
		b.Numero,
	))
}

// Client Entity that encapsulates how
type Client struct {
	config ClientConfig
	conn   net.Conn
	quit   chan os.Signal
}

// NewClient Initializes a new client receiving the configuration
// as a parameter
func NewClient(config ClientConfig) *Client {
	client := &Client{
		config: config,
		quit:   make(chan os.Signal, 1),
	}
	signal.Notify(client.quit, syscall.SIGINT, syscall.SIGTERM)
	return client
}

// CreateClientSocket Initializes client socket. In case of
// failure, error is printed in stdout/stderr and exit 1
// is returned
func (c *Client) createClientSocket() error {
	conn, err := net.Dial("tcp", c.config.ServerAddress)
	if err != nil {
		log.Criticalf(
			"action: connect | result: fail | client_id: %v | error: %v",
			c.config.ID,
			err,
		)
	}
	c.conn = conn
	return nil
}

func (c *Client) sendBet(bet *Bet) error {
	data := bet.serialize()
	totalBytes := len(data)

	header := fmt.Sprintf("%04d", totalBytes)
	if _, err := c.conn.Write([]byte(header)); err != nil {
		return err
	}

	sent := 0
	for sent < totalBytes {
		n, err := c.conn.Write(data[sent:])
		if err != nil {
			return err
		}
		sent += n
	}
	return nil
}

func (c *Client) recvResponse() error {
	respHeader := make([]byte, 4)
	if _, err := c.conn.Read(respHeader); err != nil {
		return err
	}

	respSize, err := strconv.Atoi(string(respHeader))
	if err != nil {
		return err
	}

	respData := make([]byte, respSize)
	if _, err := c.conn.Read(respData); err != nil {
		return err
	}

	responseParts := strings.Split(string(respData), ";")
	if len(responseParts) != 2 {
		return fmt.Errorf("invalid response")
	}

	documento := responseParts[0]
	numero := responseParts[1]

	log.Infof("action: apuesta_enviada | result: success | dni: %s | numero: %s",
		documento,
		numero,
	)
	return nil
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	// There is an autoincremental msgID to identify every message sent
	// Messages if the message amount threshold has not been surpassed
	for msgID := 1; msgID <= c.config.LoopAmount; msgID++ {
		// Create the connection the server in every loop iteration. Send an
		select {
		case <-c.quit:
			c.closeClient()
			return
		default:
			if err := c.createClientSocket(); err != nil {
				return
			}

			// TODO: Modify the send to avoid short-write
			if err := c.sendBet(&c.config.Bet); err != nil {
				log.Errorf("action: send_message | result: fail | client_id: %v | error: %v",
					c.config.ID,
					err,
				)
			}

			if err := c.recvResponse(); err != nil {
				log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v",
					c.config.ID,
					err,
				)
			}
			// Wait a time between sending one message and the next one
			time.Sleep(c.config.LoopPeriod)
		}
	}
	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}

func (c *Client) closeClient() {
	if c.conn != nil {
		log.Infof("action: exit | result: success | client_id: %v", c.config.ID)
		c.conn.Close()
	}
}
