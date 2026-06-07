from machine import Pin
from time import sleep
import dht

# Initialize the DHT11 sensor on GPIO 28
sensor = dht.DHT11(Pin(28))
start_time = 0
while True:
    try:
        sensor.measure()

        temp = sensor.temperature() # Celcius
        hum = sensor.humidity()     # Relative Humidity %
        print(f"{temp};{hum}")

    except OSError as e:
        print("ERROR: Failed to read data from DHT11 sensor.")

    sleep(2)
