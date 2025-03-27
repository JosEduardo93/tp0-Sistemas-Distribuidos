import sys

def main(output_file, clients):
    base_compose = f"""
    name: tp0
    services:
        server:
            container_name: server
            image: server:latest
            entrypoint: python3 /main.py
            environment:
                - PYTHONUNBUFFERED=1
                - CLIENTS={clients}
            volumes:
                - ./server/config.ini:/config.ini:ro
            networks:
                - testing_net
    """

    clients_compose = ""
    for i in range(1, clients + 1):
        clients_compose += f"""
        client{i}:
            container_name: client{i}
            image: client:latest
            entrypoint: /client
            environment:
                - CLI_ID={i}
            volumes:
                - ./client/config.yaml:/config.yaml:ro
                - ./.data/agency-{i}.csv:/agency-{i}.csv:ro
            networks:
                - testing_net
            depends_on:
                - server
        """

    network_compose = """
    networks:
        testing_net:
            ipam:
                driver: default
                config:
                    - subnet: 172.25.125.0/24
    """

    with open(output_file, "w") as f:
        f.write(base_compose + clients_compose + network_compose)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python3 generar-compose.py <output_file> <clients>")
        sys.exit(1)

    output_file = sys.argv[1]
    clients = int(sys.argv[2])

    main(output_file, clients)