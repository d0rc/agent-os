#!/usr/bin/env bash
docker run -p 6333:6333 -v `pwd`/qdrant:/qdrant/storage qdrant/qdrant
