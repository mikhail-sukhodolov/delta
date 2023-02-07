#!/bin/bash

grpcui ${GRPCUI_ARGS} -plaintext -port ${GRPCUI_PORT:-8080} -bind 0.0.0.0 ${GRPCUI_SERVER:-}