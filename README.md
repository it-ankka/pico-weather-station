# Pico weather station

An extremely minimalist Raspberry pi Pico weather station for measuring humidity and temperature.

## Components
- Raspberry pi Pico
- DHT11 sensor
- Some wires
- Micro-USB

## Requirements
- Python3
- pip
- uv

## Getting started


### Pico firmware

Run the commands inside the pico directory

Install the dependencies
```shell
uv sync
```

Install typings
```shell
uv pip install -r pyproject.toml --extra stubs --target typings
```

Flash the firmware to the Pico. Replace the PICOTTY with the correct tty name.
```shell
uv run ampy --port /dev/PICOTTY put main.py
```

### Server

Add your account to the allowed users to read dialout
```shell
sudo usermod -aG dialout $USER
```

Start the server
```shell
go run main.go -port 8080 -serialport /dev/PICOTTY 
```

Compile the application
```shell
go build -o weather-server main.go
```

Cross compile the application
```shell
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o weather-server-arm64 main.go
```
