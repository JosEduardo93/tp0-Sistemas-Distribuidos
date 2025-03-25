package common

import (
	"bufio"
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

const MAX_BATCH_SIZE = 8 * 1024

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
	MaxAmount     int
	// Bet           Bet
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

func (c *Client) sendBatches(data []byte) error {
	totalBytes := len(data)

	header := fmt.Sprintf("%04d", totalBytes)
	if _, err := c.conn.Write([]byte(header)); err != nil {
		log.Criticalf("action: send_message_header | result: fail | error: %v", err)
		return err
	}

	sent := 0
	for sent < totalBytes {
		n, err := c.conn.Write(data[sent:])
		if err != nil {
			log.Criticalf("action: send_message | result: fail | error: %v", err)
			return err
		}
		sent += n
	}
	return nil
}

func (c *Client) recvResponse() ([]byte, error) {
	respHeader := make([]byte, 4)
	if _, err := c.conn.Read(respHeader); err != nil {
		log.Criticalf("action: read_response_header | result: fail | error: %v", err)
		return nil, err
	}

	respSize, err := strconv.Atoi(string(respHeader))
	if err != nil {
		log.Criticalf("action: parse_response_header | result: fail | error: %v", err)
		return nil, err
	}

	respData := make([]byte, respSize)
	if _, err := c.conn.Read(respData); err != nil {
		log.Criticalf("action: read_response | result: fail | error: %v", err)
		return nil, err
	}

	return respData, nil
}

func readBets(c *Client) ([]*Bet, error) {
	filename := fmt.Sprintf("agency-%s.csv", c.config.ID)
	file, err := os.Open(filename)
	if err != nil {
		log.Criticalf("action: open_file | result: fail | error: %v", err)
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var bets []*Bet

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			log.Criticalf("action: read_line | result: fail | error: %v", err)
			return nil, err
		}

		line_len := strings.Split(strings.TrimSpace(line), ",")
		if len(line_len) != 5 {
			log.Criticalf("action: read_line | result: fail | error: invalid line format")
			return nil, fmt.Errorf("invalid line format")
		}

		id, _ := strconv.Atoi(c.config.ID)
		documento, _ := strconv.Atoi(line_len[2])
		numero, _ := strconv.Atoi(line_len[4])

		bet := &Bet{
			Id:         id,
			Nombre:     line_len[0],
			Apellido:   line_len[1],
			Documento:  documento,
			Nacimiento: line_len[3],
			Numero:     numero,
		}

		bets = append(bets, bet)
	}
	return bets, nil
}

func createBatches(c *Client, bets []*Bet) [][]byte {
	var batches [][]byte
	var currentBatch []byte
	betCount := 0

	for _, bet := range bets {
		betData := bet.serialize()

		if len(betData)+len(currentBatch)+1 > MAX_BATCH_SIZE || betCount >= c.config.MaxAmount {
			currentBatch = append(currentBatch, '\n')
			batches = append(batches, currentBatch)
			currentBatch = []byte{}
			betCount = 0
		}

		if len(currentBatch) > 0 {
			currentBatch = append(currentBatch, '|')
		}
		currentBatch = append(currentBatch, betData...)
		betCount++
	}

	if len(currentBatch) > 0 {
		currentBatch = append(currentBatch, '\n')
		batches = append(batches, currentBatch)
	}
	return batches
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	// Open file and send messages to the server
	bets, err := readBets(c)
	if err != nil {
		log.Criticalf("action: read_bets | result: fail | error: %v", err)
		return
	}
	// There is an autoincremental msgID to identify every message sent
	// Messages if the message amount threshold has not been surpassed

	batches := createBatches(c, bets)
	if err := c.createClientSocket(); err != nil {
		log.Criticalf("action: create_socket | result: fail | error: %v", err)
		return
	}
	defer c.closeClient()

	for _, batch := range batches {
		// Create the connection the server in every loop iteration. Send an
		select {
		case <-c.quit:
			c.closeClient()
			return
		default:
			// TODO: Modify the send to avoid short-write
			if err := c.sendBatches(batch); err != nil {
				log.Errorf("action: send_message | result: fail | client_id: %v | error: %v",
					c.config.ID,
					err,
				)
				return
			}

			response, err := c.recvResponse()
			if err != nil {
				log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v",
					c.config.ID,
					err,
				)
				return
			}

			c.parseResponse(response)
			// Wait a time between sending one message and the next one
			time.Sleep(c.config.LoopPeriod)
		}
	}

	endSignal := []byte("END\n")
	if err := c.sendBatches(endSignal); err != nil {
		log.Errorf("action: send_message | result: fail | client_id: %v | error: %v",
			c.config.ID,
			err,
		)
		return
	}

	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}

func (c *Client) parseResponse(response []byte) {
	responseParts := strings.Split(string(response), ";")
	if len(responseParts) != 2 {
		log.Criticalf("action: parse_response | result: fail | client_id: %v | error: invalid response format",
			c.config.ID,
		)
		return
	}

	code := responseParts[0]

	lenght, err := strconv.Atoi(responseParts[1])
	if err != nil {
		log.Criticalf("action: parse_response | result: fail | error: %v", err)
		return
	}

	switch code {
	case "FAIL":
		log.Errorf("action: apuesta_recibida | result: fail | cantidad: %d", lenght)
	// case "SUCCESS":
	// 	log.Infof("action: apuesta_recibida | result: success | cantidad: %d", lenght)
	default:
		log.Criticalf("action: parse_response | result: fail | client_id: %v | error: invalid response code",
			c.config.ID,
		)
		return
	}
}

func (c *Client) closeClient() {
	if c.conn != nil {
		log.Infof("action: exit | result: success | client_id: %v", c.config.ID)
		c.conn.Close()
	}
}
