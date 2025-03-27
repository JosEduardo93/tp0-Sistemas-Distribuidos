#!/bin/bash

#Verificar si los argumentos son correctos
if [ "$#" -ne 2 ]; then
    # echo "Error: Numero de argumentos incorrecto"
    echo "Uso: $0 <file_name> <amount_clients>"
    exit 1
fi

FILENAME=$1
AMOUNT_CLIENTS=$2

if [ $AMOUNT_CLIENTS -lt 1 ]; then
    echo "Advertencia: La cantidad de clientes debe ser mayor o igual a 1. Se usará 1 como valor por defecto."
    AMOUNT_CLIENTS=1
fi

# Crear el archivo docker-compose.yaml
echo "Generando archivo docker compose: $FILENAME con $AMOUNT_CLIENTS clientes..."
python3 generator-compose.py $FILENAME $AMOUNT_CLIENTS
echo "Archivo docker-compose.yaml generado con exito"