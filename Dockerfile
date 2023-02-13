FROM dyne/devuan:chimaera AS zenroom
RUN apt update && apt install -y build-essential git cmake vim python3 python3-pip zsh \
        && pip3 install meson ninja \
        && git clone https://github.com/dyne/Zenroom.git /zenroom
RUN cd /zenroom && make linux-go

FROM golang:1.19-bullseye AS builder
ENV GONOPROXY=
RUN apt update && apt install -y libssl-dev
COPY --from=zenroom /zenroom/meson/libzenroom.so /usr/lib/
COPY --from=zenroom /usr/lib/x86_64-linux-gnu/libssl.so.1.1 /lib/
COPY --from=zenroom /usr/lib/x86_64-linux-gnu/libcrypto.so.1.1 /lib/
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download && go mod verify

ADD . .
RUN go build -o inbox .

FROM dyne/devuan:chimaera
WORKDIR /root
ENV HOST=0.0.0.0
ENV PORT=80
EXPOSE 80
COPY --from=builder /app/inbox /root/
COPY --from=zenroom /zenroom/meson/libzenroom.so /usr/lib/
COPY --from=zenroom /usr/lib/x86_64-linux-gnu/libssl.so.1.1 /lib/
COPY --from=zenroom /usr/lib/x86_64-linux-gnu/libcrypto.so.1.1 /lib/
CMD ["/root/inbox"]
