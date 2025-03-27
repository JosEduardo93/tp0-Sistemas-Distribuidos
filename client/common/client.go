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

const (
	MAX_BATCH_SIZE       = 8 * 1024
	CODE_AGENCY          = 'A'
	CODE_BATCH           = 'B'
	CODE_WAIT_FOR_RESULT = 'W'
	CODE_RESULT          = 'R'
	CODE_END             = 'E'
	CODE_WINNER          = 'S'
	SIZE_HEADER          = 4
)

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
	MaxAmount     int
	Phase         int
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

func (c *Client) sendOne(b byte) int {
	n, err := c.conn.Write([]byte{b})
	if err != nil {
		log.Criticalf("action: send_message | result: fail | error: %v", err)
		return 0
	}
	return n
}

func (c *Client) send(data []byte, size int) int {
	totalSend := 0
	for totalSend < size {
		n := c.sendOne(data[totalSend])
		if n == 0 {
			return totalSend
		}
		totalSend += n
	}
	return totalSend
}

func (c *Client) recvOne() (byte, error) {
	buf := make([]byte, 1)
	_, err := c.conn.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (c *Client) recv(size int) []byte {
	data := make([]byte, size)
	totalRecv := 0
	for totalRecv < size {
		b, err := c.recvOne()
		if err != nil {
			log.Criticalf("action: receive_message | result: fail | error: %v", err)
			return nil
		}
		data[totalRecv] = b
		totalRecv++
	}
	return data
}

func (c *Client) sendBatch(data []byte) bool {
	totalBytes := len(data)

	header := fmt.Sprintf("%04d", totalBytes)
	n := c.send([]byte(header), SIZE_HEADER)
	if n != SIZE_HEADER {
		log.Errorf("action: send_message_header | result: fail | client_id: %v | error: invalid size",
			c.config.ID,
		)
		return true
	}
	totalSend := c.send(data, totalBytes)
	if totalSend != totalBytes {
		log.Errorf("action: send_message_data | result: fail | client_id: %v | error: invalid data",
			c.config.ID,
		)
		return true
	}
	return false
}

func (c *Client) recvResponse() []byte {
	respHeader := c.recv(SIZE_HEADER)
	if respHeader == nil {
		log.Criticalf("action: read_response_header | result: fail")
		return nil
	}

	respSize, err := strconv.Atoi(string(respHeader))
	if err != nil {
		log.Criticalf("action: parse_response_header | result: fail | error: %v", err)
		return nil
	}

	respData := c.recv(respSize)
	if respData == nil {
		log.Criticalf("action: read_response | result: fail | error: %v", err)
		return nil
	}

	return respData
}

func readBet(c *Client, reader *bufio.Reader) (*Bet, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if err.Error() == "EOF" {
			return nil, nil
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
	return bet, nil
}

func (c *Client) createBatch(reader *bufio.Reader) ([]byte, bool) {
	var batchData []byte
	betCount := 0
	eof := false

	for betCount < c.config.MaxAmount {
		bet, err := readBet(c, reader)
		if err != nil {
			log.Criticalf("action: read_bet | result: fail | error: %v", err)
			return nil, true
		}
		if bet == nil {
			eof = true
			break
		}

		serializeBet := bet.serialize()

		if len(batchData) > 0 && len(batchData)+len(serializeBet)+1 > MAX_BATCH_SIZE {
			break
		}

		if len(batchData) > 0 {
			batchData = append(batchData, '|')
		}

		batchData = append(batchData, serializeBet...)
		betCount++
	}

	if len(batchData) > 0 {
		batchData = append(batchData, '\n')
	}
	return batchData, eof
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	// Open file and send messages to the server
	filename := fmt.Sprintf("agency-%s.csv", c.config.ID)
	file, err := os.Open(filename)
	if err != nil {
		log.Criticalf("action: open_file | result: fail | error: %v", err)
		return
	}
	defer file.Close()

	fileReader := bufio.NewReader(file)

	// There is an autoincremental msgID to identify every message sent
	// Messages if the message amount threshold has not been surpassed

	if err := c.createClientSocket(); err != nil {
		log.Criticalf("action: create_socket | result: fail | error: %v", err)
		return
	}
	defer c.closeClient()

	// finishedLotery := false

	// Send the code of the agency
	// n := c.sendOne(CODE_AGENCY)
	// if n == 0 {
	// 	log.Criticalf("action: send_message_code_agency | result: fail | client: %v", c.config.ID)
	// 	return
	// }
	// if error := c.sendCodeAgency(); error {
	// 	log.Errorf("action: send_code_agency | result: fail | client: %v", c.config.ID)
	// 	return
	// }

	for {
		// 	// Create the connection the server in every loop iteration. Send an
		select {
		case <-c.quit:
			c.closeClient()
			return
		default:
			finishedLotery := c.handlePhase(fileReader)
			if finishedLotery {
				log.Infof("action: finished_lotery | result: success")
				return
			}
			// Wait a time between sending one message and the next one
			time.Sleep(c.config.LoopPeriod)
		}
	}
}

func (c *Client) sendCodeAgency() bool {
	id, err := strconv.Atoi(c.config.ID)
	if err != nil {
		log.Criticalf("action: convert_id | result: fail | error: %v", err)
		return true
	}
	data := []byte(fmt.Sprintf("%01d", id))
	n := c.send(data, 1)
	if n == 0 {
		log.Criticalf("action: send_message_code_agency | result: fail")
		return true
	}
	return false
}

func (c *Client) handlePhase(reader *bufio.Reader) bool {
	switch c.config.Phase {
	case CODE_BATCH:
		c.handleBatch(reader)
	case CODE_RESULT:
		c.handleResult()
	case CODE_WAIT_FOR_RESULT:
		c.handleWaitForResult()
	case CODE_END:
		c.handleCloseConnection()
		return true
	default:
		log.Criticalf("action: handle_phase | result: fail | client_id: %v | error: invalid phase",
			c.config.ID,
		)
	}
	return false
}

func (c *Client) handleBatch(reader *bufio.Reader) {
	batch, eof := c.createBatch(reader)
	if eof {
		c.config.Phase = CODE_RESULT
		return
	}

	n := c.sendOne(CODE_BATCH)
	if n == 0 {
		log.Errorf("action: send_message_code_batch | result: fail | client_id: %v",
			c.config.ID,
		)
		return
	}

	if error := c.sendBatch(batch); error {
		log.Errorf("action: send_message_batch | result: fail | client_id: %v",
			c.config.ID,
		)
		return
	}

	response := c.recvResponse()
	if response == nil {
		log.Errorf("action: receive_status_batch | result: fail | client_id: %v ",
			c.config.ID,
		)
		return
	}

	c.parseResponse(response)
}

func (c *Client) handleWaitForResult() {
	code, err := c.recvOne()
	if err != nil {
		log.Criticalf("action: read_code | result: fail | error: %v", err)
		return
	}
	if code == CODE_WAIT_FOR_RESULT {
		c.handleCloseConnection()
		time.Sleep(5 * time.Second)
		c.closeClient()
		if err := c.createClientSocket(); err != nil {
			log.Criticalf("action: create_socket | result: fail | error: %v", err)
			return
		}
		c.config.Phase = CODE_RESULT
		return
	}
	listWinner := c.recvWinners()
	if listWinner == nil {
		log.Criticalf("action: read_winners | result: fail")
		return
	}
	listWinnerStr := strings.Split(string(listWinner), ";")
	log.Infof("action: consulta_ganadores | result: success | cant_ganadores: %d", len(listWinnerStr))
	c.config.Phase = CODE_END
}

func (c *Client) handleResult() {
	n := c.send([]byte{CODE_RESULT}, 1)
	if n != 1 {
		log.Criticalf("action: send_message_result | result: fail")
		return
	}
	error := c.sendCodeAgency()
	if error {
		log.Criticalf("action: send_code_agency | result: fail")
		return
	}
	c.config.Phase = CODE_WAIT_FOR_RESULT
}

func (c *Client) handleCloseConnection() {
	n := c.sendOne(CODE_END)
	if n == 0 {
		log.Criticalf("action: send_message_end | result: fail")
	}
	log.Infof("action: close_connection | result: success")
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
		return
	// case "SUCCESS":
	// 	log.Infof("action: apuesta_recibida | result: success | cantidad: %d", lenght)
	default:
		// log.Criticalf("action: parse_response | result: fail | client_id: %v | error: invalid response code",
		// 	c.config.ID,
		// )
		return
	}
}

func (c *Client) recvWinners() []byte {
	sizeData := c.recv(SIZE_HEADER)
	if sizeData == nil {
		log.Criticalf("action: read_winners_header | result: fail")
		return nil
	}

	size, err := strconv.Atoi(string(sizeData))
	if err != nil {
		log.Criticalf("action: parse_winners_header | result: fail | error: %v", err)
		return nil
	}

	data := c.recv(size)
	if data == nil {
		log.Criticalf("action: read_winners | result: fail")
		return nil
	}

	return data
}

func (c *Client) closeClient() {
	if c.conn != nil {
		log.Infof("action: exit | result: success | client_id: %v", c.config.ID)
		time.Sleep(1 * time.Second)
		c.conn.Close()
	}
}
