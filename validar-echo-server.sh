#!/bin/bash

# Configuracion
SERVER_CONTAINER="server"
SERVER_PORT=12345
TEST_MESSAGE="Hello, echo Server!"
SERVER_NETWORK="tp0_testing_net"

RESPONSE=$(docker run --rm --network=$SERVER_NETWORK busybox sh -c "printf '$TEST_MESSAGE\n' | nc -w 1 $SERVER_CONTAINER $SERVER_PORT")

if [[ "$RESPONSE" == "$TEST_MESSAGE" ]]; then
    echo "action: test_echo_server | result: success"
else
    echo "action: test_echo_server | result: fail"
    echo "expected: $TEST_MESSAGE"
    echo "received: $RESPONSE"
fi