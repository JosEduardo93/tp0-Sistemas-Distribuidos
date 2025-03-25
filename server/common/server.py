import socket
import logging
import signal
from common import utils

class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self.server_is_alive = True

    def run(self):
        """
        Dummy Server loop

        Server that accept a new connections and establishes a
        communication with a client. After client with communucation
        finishes, servers starts to accept new connections again
        """

        # TODO: Modify this program to handle signal to graceful shutdown
        # the server

        signal.signal(signal.SIGTERM, self.__signal_handler)
        signal.signal(signal.SIGINT, self.__signal_handler)

        while self.server_is_alive:
            try:
                client_sock = self.__accept_new_connection()
                if client_sock:
                    self.__handle_client_connection(client_sock)
            except OSError as e:
                logging.error(f"action: accept_connections | result: fail | error: {e}")
                break

        self.__close_server()

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket

        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            # TODO: Modify the receive to avoid short-reads
            addr = client_sock.getpeername()

            while True:
                (bets, batch_failed) = self.recv_batches(client_sock)
                if not bets:
                    break
                utils.store_bets(bets)
                response = ''

                if batch_failed > 0:
                    logging.error(f"action: apuesta_recibida | result: fail | cantidad: {batch_failed}")
                    response = f'FAIL;{batch_failed}'.encode('utf-8')
                else:
                    logging.info(f"action: apuesta_recibida | result: success | cantidad: {len(bets)}")
                    response = f'SUCCESS;{len(bets)}'.encode('utf-8')

                response_len = f"{len(response):04d}".encode('utf-8')
                self.__send_all(client_sock, response_len)
                self.__send_all(client_sock, response)
                # all_bets.extend(bets)
                # failed_bets += batch_failed
                if not bets and batch_failed == 0:
                    break

        
        except OSError as e:
            logging.error("action: receive_message | result: fail | error: {e}")
        finally:
            client_sock.close()

    def recv_batches(self, client_sock) -> tuple[list, int]:
        header = self.__recv_all(client_sock, 4)
        if not header:
            logging.error(f"action: receive_message | result: fail | error: short-read")
            return None, 0
        
        buffer = bytearray()
        message_size = int(header)
        received_bytes = 0
        failed_bets = 0
        bets = []

        while received_bytes < message_size:
            chunk = client_sock.recv(min(1024, message_size - received_bytes))
            if not chunk:
                logging.error(f"action: receive_message | result: fail | error: connection-lost")
                return None, 0
            buffer.extend(chunk)
            received_bytes += len(chunk)
            
        batch_data = buffer.decode('utf-8').strip()
        batch_list = batch_data.split('\n')

        for batch in batch_list:
            if batch.strip() == "END":
                logging.info(f"action: receive_end_batch | result: success ")
                break
            batch_bets = batch.split('|')
            for b in batch_bets:
                bet = b.split(';')
                if len(bet) == 6:
                    bets.append(utils.Bet(*bet))
                else:
                    failed_bets += 1
        return bets, failed_bets
    
    def __recv_all(self, sock, size):
        data = b''
        while len(data) < size:
            try:
                chunk = sock.recv(size - len(data))
                if not chunk:
                    raise RuntimeError("Socket connection broken")
                data += chunk
            except OSError as e:
                logging.error(f"action: receive_message | result: fail | error: {e}")
                return None
        return data

    def __send_all(self, sock, data):
        total_sent = 0
        while total_sent < len(data):
            try:
                sent = sock.send(data[total_sent:])
                if sent == 0:
                    raise RuntimeError("Socket connection broken")
                total_sent += sent
            except OSError as e:
                logging.error(f"action: send_message | result: fail | error: {e}")
                return False
        return True

    def __accept_new_connection(self):
        """
        Accept new connections

        Function blocks until a connection to a client is made.
        Then connection created is printed and returned
        """
        # Connection arrived
        try:
            logging.info('action: accept_connections | result: in_progress')
            c, addr = self._server_socket.accept()
            logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
            return c
        except OSError:
            return None

    def __signal_handler(self, signum, frame):
        signame = signal.Signals(signum).name
        logging.info(f"action: exit | result: success | signal: {signame}")
        self.server_is_alive = False