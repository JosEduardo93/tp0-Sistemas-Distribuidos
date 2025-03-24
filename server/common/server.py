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

            msg_str = self.recv_bets(client_sock)            
            # TODO: Modify the send to avoid short-writes
            bets = msg_str.split('|')
            result_bets = []
            for bet in bets:
                bet = bet.split(';')
                result_bets.append(utils.Bet(bet[0], bet[1], bet[2], bet[3], bet[4], bet[5]))
            utils.store_bets(result_bets)

            logging.info(f"action: apuesta_recibida | result: success | cantidad: {len(bets)}")

            response = f"OK;{len(result_bets)}".encode('utf-8')
            response_len = f"{len(response):04d}".encode('utf-8')
            if not self.__send_all(client_sock, response_len):
                return
            
            if not self.__send_all(client_sock, response):
                return

        except OSError as e:
            logging.error("action: receive_message | result: fail | error: {e}")
        finally:
            client_sock.close()

    def recv_bets(self, client_sock):
        # Receive the number of length of the message
        header = self.__recv_all(client_sock, 4)
        if not header:
            logging.error(f"action: receive_message | result: fail | error: short-read")
            return
        
        message_size = int(header)
        logging.info(f'action: receive_message | result: success | msg_len: {message_size}')

        # Receive the message bet
        full_msg = self.__recv_all(client_sock, message_size)
        if not full_msg:
            logging.error(f"action: receive_message | result: fail | error: short-read")
            return
        
        msg_str = full_msg.decode('utf-8').rstrip()
        return msg_str

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