import serial
import json
import threading
import argparse
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

parser = argparse.ArgumentParser(
        prog="Pico weather HTTP-server",
        description="Sets up an HTTP-server for viewing pico weather station information"
        )

parser.add_argument("serialport", default="/dev/ttyACM0", type=str, nargs="?")
parser.add_argument("-r", "--rate", default=115200, type=int)
parser.add_argument("-p", "--port", default=8080, type=int)
parser.add_argument("-v", "--verbose")

args = parser.parse_args()

serialport: str = args.serialport
rate: int = args.rate
port: int = args.port
verbose: bool = args.verbose

data_lock = threading.Lock()

measurements: list[tuple[int, int]] = []
cur_average: tuple[float, float] = (0.0, 0.0)
last_measurement_timestamp: float = 0.0

def get_averages(measurements_list: list[tuple[int, int]], n=60):
    if not measurements_list:
        return (0.0, 0.0)
    subset = measurements_list[-n:]
    avg_temp = sum([m[0] for m in subset]) / len(subset)
    avg_hum = sum([m[1] for m in subset]) / len(subset)
    return (round(avg_temp, 2), round(avg_hum, 2))

def pico_serial_reader():
    # Declare global to allow reassignment/slicing of the list variable
    global measurements, cur_average, last_measurement_timestamp
    print(f"Connecting to Pico on {serialport}...")
    try:
        with serial.Serial(serialport, rate, timeout=1) as ser:
            print(f"Connected to serial port {serialport}.")
            ser.reset_input_buffer()

            while True:
                if ser.in_waiting > 0:
                    raw_line = ser.readline()
                    try:
                        clean_line = raw_line.decode('utf-8').strip()
                        if clean_line.startswith("ERROR") or not clean_line:
                            print(clean_line, file=sys.stderr)
                            continue
                        
                        [temp, hum] = [int(m) for m in clean_line.split(";")]
                        
                        with data_lock:
                            measurements.append((temp, hum))
                            measurements = measurements[-1000:]
                            cur_average = get_averages(measurements, n=60)
                            last_measurement_timestamp = time.time()
                            if verbose:
                                print(f"Temperature: {temp}°C\tHumidity: {hum}%")
                        
                    except (UnicodeDecodeError, ValueError):
                        continue
                time.sleep(0.01)
                        
    except serial.SerialException as e:
        print(f"Error opening serial port: {e}", file=sys.stderr)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        user_agent = self.headers.get('User-Agent', '')
        accept_header = self.headers.get('Accept', '')

        with data_lock:
            latest = measurements[-1] if measurements else (None, None)
            avg_temp, avg_hum = cur_average
            last_seen = last_measurement_timestamp

        data_payload = {
            "status": "online" if measurements else "no_data",
            "last_updated_epoch": last_seen,
            "latest": {
                "temperature": latest[0],
                "humidity": latest[1]
            },
            "rolling_average_2m": {
                "temperature": avg_temp,
                "humidity": avg_hum
            }
        }

        if 'curl' in user_agent.lower() or 'json' in accept_header.lower():
            self.send_response(200)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.end_headers()
            
            response_json = json.dumps(data_payload, indent=2) + "\n"
            self.wfile.write(response_json.encode('utf-8'))
        else:
            self.send_response(200)
            self.send_header('Content-Type', 'text/html; charset=utf-8')
            self.end_headers()
            
            html_content = f"""
            <!DOCTYPE html>
            <html>
            <head>
                <title>Pico Weather Station</title>
                <meta http-equiv="refresh" content="5">
                <style>
                    body {{ font-family: sans-serif; margin: 40px; background: #f4f6f9; color: #333; }}
                    .card {{ background: white; padding: 20px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); max-width: 500px; }}
                    h1 {{ color: #2c3e50; margin-top: 0; }}
                    .metric {{ font-size: 1.2em; margin: 10px 0; }}
                    .val {{ font-weight: bold; color: #2980b9; }}
                </style>
            </head>
            <body>
                <div class="card">
                    <h1>Pico Weather Station</h1>
                    <div class="metric">Latest Temperature: <span class="val">{data_payload['latest']['temperature'] or 'N/A'}°C</span></div>
                    <div class="metric">Latest Humidity: <span class="val">{data_payload['latest']['humidity'] or 'N/A'}%</span></div>
                    <hr>
                    <div class="metric">2-Min Avg Temp: <span class="val">{avg_temp}°C</span></div>
                    <div class="metric">2-Min Avg Humidity: <span class="val">{avg_hum}%</span></div>
                    <p style="font-size:0.8em; color:#7f8c8d;">Last updated: {time.ctime(last_seen) if last_seen else 'Never'}</p>
                </div>
            </body>
            </html>
            """
            self.wfile.write(html_content.encode('utf-8'))

if __name__ == '__main__':
    serial_thread = threading.Thread(target=pico_serial_reader, daemon=True)
    serial_thread.start()

    server_address = ('', port)
    httpd = HTTPServer(server_address, Handler)
    print(f"Server running on http://localhost:{port} ...")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down server.")
        httpd.server_close()
