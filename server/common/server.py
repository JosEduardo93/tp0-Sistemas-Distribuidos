import socket
import logging
import signal
from common import utils

MAX_AGENCIES = 2
CODE_AGENCY = b'A'
CODE_BATCH = b'B'
CODE_RESULT = b'R'
CODE_END = b'E'

class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self.server_is_alive = True
        self.list_clients = {}
        self.list_winners = {}
        self.waiting_clients = {}

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
            logging.info(f"action: receive_message | result: in_progress | ip: {addr[0]}")

            endClient = False
            while not endClient:
                code = self.__recv_all(client_sock, 1)
                if code == CODE_END:
                    logging.info(f"action: end_connection | result: success | ip: {client_sock.getpeername()[0]}")
                    endClient = True                
                else:
                    self.handle_client_connection(client_sock, code)

        except OSError as e:
            logging.error("action: receive_message | result: fail | error: {e}")
        finally:
            client_sock.close()

    def handle_client_connection(self, client_sock, code):
        if code == CODE_AGENCY:
            self.__handle_agency(client_sock)
        elif code == CODE_BATCH:
            self.__handle_batch(client_sock)
        elif code == CODE_RESULT:
            self.__handle_result(client_sock)

    def __handle_agency(self, client_sock):
        sizeData = self.__recv_all(client_sock, 4)
        sizeData = int(sizeData)
        idAgency = self.__recv_all(client_sock, sizeData)
        data = int(idAgency)
        logging.info(f"action: receive_agency | result: success | id: {data}")
        self.list_clients[client_sock.getpeername()] = [data, client_sock]        

    def __handle_batch(self, client_sock):
        (batch, failed_bets) = self.recv_batch(client_sock)
        if failed_bets > 0:
            logging.error(f"action: receive_batch | result: fail | error: {failed_bets}")
            response = f'FAIL;{len(batch)}'.encode('utf-8')
        else:
            logging.info(f"action: receive_batch | result: success | cantidad: {len(batch)}")
            response = f'SUCCESS;{len(batch)}'.encode('utf-8')
        utils.store_bets(batch)
        response_len = f"{len(response):04d}".encode('utf-8')
        self.__send_all(client_sock, response_len)
        self.__send_all(client_sock, response)

    def __handle_result(self, client_sock):
        agency_id = self.list_clients.get(client_sock.getpeername())
        if not agency_id:
            logging.error(f"action: receive_result | result: fail | error: unknown agency")
            return
        
        self.list_winners[agency_id] = True
        self.waiting_clients[agency_id] = client_sock

        if len(self.list_winners) < MAX_AGENCIES:
            logging.info(f"action: wait_for_sorteo | result: in_progress | agencies: {len(self.list_winners)}/{MAX_AGENCIES}")
            return

        logging.info("action: sorteo | result: success")

        all_bets = utils.load_bets()
        self.winners = {}

        for bet in all_bets:
            if utils.has_won(bet):
                if bet.agency not in self.winners:
                    self.winners[bet.agency] = []
                self.winners[bet.agency].append(bet.document)

        # Ahora que el sorteo está completo, enviamos los resultados a los clientes en espera
        for agency_id, client in self.waiting_clients.items():
            self.__send_winners(client, agency_id)
        self.waiting_clients.clear()

    def __send_winners(self, client_sock, agency_id):
        winners_list = self.winners.get(agency_id, [])
        winners_data = ";".join(map(str, winners_list)).encode("utf-8")

        response_len = f"{len(winners_data):04d}".encode('utf-8')
        self.__send_all(client_sock, response_len)
        self.__send_all(client_sock, winners_data)

        logging.info(f"action: send_winners | result: success | agency: {agency_id} | cant_ganadores: {len(winners_list)}")


    def recv_batch(self, client_sock) -> tuple[list, int]:
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
            chunk = client_sock.recv(1024)
            if not chunk:
                logging.error(f"action: receive_message | result: fail | error: connection-lost")
                return None, 0
            buffer.extend(chunk)
            received_bytes += len(chunk)
            
        batch_data = buffer.decode('utf-8').strip()
        batch_list = batch_data.split('\n')

        for batch in batch_list:
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
                    return None
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
                    return False
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