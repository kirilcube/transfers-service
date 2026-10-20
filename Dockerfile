FROM ubuntu:latest
LABEL authors="kiril"

ENTRYPOINT ["top", "-b"]