#include <WiFiClientSecure.h>
#include <WiFiClient.h>
#include "config.h"

#ifndef API_SECRET_KEY
  #define API_SECRET_KEY "dev_API_SECRET_KEY"
#endif

#ifndef API_PATH
  #define API_PATH "/gate";
#endif

#ifdef SECURED
  #define PROTOCOL "https"
#else
  #define PROTOCOL "http"
#endif

const uint8_t PIN_RELAY = 13;
const uint8_t PIN_POWER = 14;

void setup() {
  //Initialize serial and wait for port to open:
  Serial.begin(115200);
  delay(1000);
  Serial.println(
    "\n\n"
    "=============\n"
    "||  START  ||\n"
    "=============\n\n");

  pinMode(PIN_RELAY, OUTPUT);
  pinMode(PIN_POWER, OUTPUT);
  digitalWrite(PIN_RELAY, LOW);
  digitalWrite(PIN_POWER, LOW);
}

void loop() {
  connectToWiFi();

#ifdef SECURED
  WiFiClientSecure client;
  client.setCACert(ssl_root_ca);
#else
  WiFiClient client;
#endif

  while (WiFi.status() == WL_CONNECTED) {
    if (!client.connect(API_DOMAIN, API_PORT)) {
      Serial.println("Connection failed! Retrying in 1s.");
      delay(5000);
      continue;
    }

    char url[512];
    sprintf(url, "%s://%s:%d%s", PROTOCOL, API_DOMAIN, API_PORT, API_PATH);

    Serial.printf("\nWaiting for open request: %s\n", url);
    client.printf(
      "GET %s HTTP/1.0\n"
      "Host: %s:%d\n"
      "Connection: close\n"
      "Authorization: %s\n"
      "\n",
      url, API_DOMAIN, API_PORT, API_SECRET_KEY
    );

    int status = 0;
    while (client.connected()) {
      Serial.printf("%d", WiFi.RSSI());
      Serial.print(".");
      String line = client.readStringUntil('\n');
      if (line.startsWith("HTTP")) {
        status = (line[9] - 48) * 100 + (line[10] - 48) * 10 + (line[11] - 48);
        Serial.printf("\nReceived status: %d\n", status);
        break;
      }
    }

    if (status == 200) {
      Serial.println("Opening the gate");
      digitalWrite(PIN_POWER, HIGH);
      delay(500);
      for (int i = 3 ; i != 0 ; i--) {
        digitalWrite(PIN_RELAY, HIGH);
        delay(500);
        digitalWrite(PIN_RELAY, LOW);
        delay(750);
      }
      digitalWrite(PIN_POWER, LOW);
      Serial.println("Gate should be open");
    } else if (status == 408) {
      Serial.println("Timeout, reconecting.");
    } else {
      Serial.printf("Unexpected status: %d\n", status);
      Serial.println("HTTP response body:");
      while (client.available()) {
        char c = client.read();
        Serial.write(c);
      }
      Serial.println();

      Serial.println("Retrying in 5s.");
      delay(5000);
    }

    client.stop();
  }
}

void connectToWiFi() {
  Serial.printf("Attempting to connect to SSID: %s\n", WIFI_SSID);
  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

  // attempt to connect to Wifi network:
  while (WiFi.status() != WL_CONNECTED) {
    Serial.print(".");
    // wait 1 second for re-trying
    delay(100);
  }

  Serial.printf("\nConnected to %s\n", WIFI_SSID);
}