#include <WiFiClientSecure.h>
#include <WiFiClient.h>
#include <WiFi.h>
#include "config.h"
#include "soc/soc.h"
#include "soc/rtc_cntl_reg.h"

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

const char *http_request_format =
  "GET %s HTTP/1.0\r\n"
  "Host: %s:%d\r\n"
  "Connection: close\r\n"
  "Authorization: %s\r\n"
  "\r\n";

void setup() {
  //Initialize serial and wait for port to open:
  Serial.begin(115200);
  delay(1000);
  Serial.println(
    "\r\n\r\n"
    "=============\r\n"
    "||  START  ||\r\n"
    "=============\r\n\r\n");

  // WRITE_PERI_REG(RTC_CNTL_BROWN_OUT_REG, 0); //disable   detector

  pinMode(PIN_RELAY, OUTPUT);
  pinMode(PIN_POWER, OUTPUT);
  digitalWrite(PIN_RELAY, LOW);
  digitalWrite(PIN_POWER, LOW);
}

void loop() {
  connectToWiFi(selectWifi());

#ifdef SECURED
  WiFiClientSecure client;
  client.setCACert(ssl_root_ca);
#else
  WiFiClient client;
#endif

  while (WiFi.status() == WL_CONNECTED) {
    if (!client.connect(API_DOMAIN, API_PORT)) {
      Serial.println("Connection failed! Retrying in 1s.");
      delay(5 * 1000);
      continue;
    }

    char url[512];
    sprintf(url, "%s://%s:%d%s", PROTOCOL, API_DOMAIN, API_PORT, API_PATH);

    Serial.printf("\r\nWaiting for open request: %s\r\n", url);
    Serial.printf(http_request_format, url, API_DOMAIN, API_PORT, API_SECRET_KEY);
    client.printf(
      http_request_format,
      url, API_DOMAIN, API_PORT, API_SECRET_KEY);

    int status = 0;
    while (client.connected()) {
      Serial.printf("\r%4d ", WiFi.RSSI());
      String line = client.readStringUntil('\r\n');
      if (line.startsWith("HTTP")) {
        status = (line[9] - 48) * 100 + (line[10] - 48) * 10 + (line[11] - 48);
        Serial.printf("\r\nReceived status: %d\r\n", status);
        break;
      }
    }

    if (status == 200) {
      Serial.println("Opening the gate");
      digitalWrite(PIN_POWER, HIGH);
      delay(500);
      for (int i = 3; i != 0; i--) {
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
      Serial.printf("Unexpected status: %d\r\n", status);
      Serial.println("HTTP response body:");
      while (client.available()) {
        char c = client.read();
        Serial.write(c);
      }
      Serial.println();

      Serial.println("Retrying in 5s.");
      delay(5 * 1000);
    }

    client.stop();
  }
}

void connectToWiFi(int wifi_id) {
  const char* ssid = wifis[wifi_id];
  const char* password = wifis[wifi_id + 1];
  Serial.printf("Attempting to connect to SSID: %s\r\n", ssid);
  WiFi.begin(ssid, password);


  uint tryies = 0;
  // attempt to connect to Wifi network:
  while (WiFi.status() != WL_CONNECTED) {
    if(tryies % 3 == 0) {
      Serial.print("\r");
    }
    Serial.print(".");
    delay(500); 
    tryies++;
  }

  Serial.printf("\r\nConnected to %s\r\n\r\n", ssid);
}

int selectWifi() {
  Serial.printf("Known WiFi configured: %d\r\n", nb_wifis);
  for (int i = 0; i < nb_wifis; i++) {
    Serial.printf("\t- %s\r\n", wifis[i * 2]);
  }

  int best_wifi = -1;

  while (true) {
    Serial.print("Scanning for available wifis...");
    delay(2*1000);
    int32_t best_signal = -1000;
    int16_t found_wifis = WiFi.scanNetworks();

    Serial.printf(" %d WiFi found:\r\n", found_wifis);

    for (int16_t i = 0; i < found_wifis; i++) {
      Serial.printf("\t- SSID: %s | RSSI: %d ", WiFi.SSID(i),  WiFi.RSSI(i));

      if (WiFi.RSSI(i) < best_signal) {
        Serial.println("(worse)");
        continue;
      }

      int found_wifi_id = findWifiId(WiFi.SSID(i));
      if(found_wifi_id != -1) {
        best_wifi = found_wifi_id;
        best_signal = WiFi.RSSI(i);
        Serial.println("(selected)");
      } else {
        Serial.println("(unknown)");
      }
    }

    if (best_wifi != -1) {
      Serial.printf("Best WiFi selected: %d %s\r\n", best_wifi, wifis[best_wifi]);
      return best_wifi;
    } else {
      Serial.println("No known wifi found... waiting 10 seconds before retrying\r\n\r\n");
      delay(10 * 1000);
    }
  }
}

int findWifiId(String ssid) {
  for (int j = 0; j < nb_wifis; j++) {
    if (strcmp(ssid.c_str(), wifis[j*2]) == 0) {
      return j*2;
    }
  }

  return -1;
}