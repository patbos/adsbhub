package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	messagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "adsb_received_messages",
		Help: "The total number of received ADS-B messages",
	})
	messagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "adsb_sent_messages",
		Help: "The total number of sent ADS-B messages",
	})

	currentIp = "0.0.0.0"
	clientKey = ""
)

func sender(host string, port int, messages chan string) {
	for {
		var address = host + ":" + strconv.Itoa(port)
		log.Printf("Trying to connect sender to %s", address)
		conn, err := net.Dial("tcp", address)
		writer := bufio.NewWriter(conn)
		if err == nil {
			log.Printf("Connected sender to %s", address)
			for {
				message := <-messages
				_, writeErr := writer.WriteString(message + "\n")
				flushErr := writer.Flush()
				if writeErr == nil && flushErr == nil {
					if message != "" {
						messagesSent.Inc()
					}
				} else {
					messages <- message
					log.Printf("Could not send message! %v %v", writeErr, flushErr)
					break
				}
			}
			closeErr := conn.Close()
			if closeErr != nil {
				log.Printf("Could not close connection %v", closeErr)
			}
		} else {
			log.Printf("Could not connect sender to %s", address)
		}
		time.Sleep(10 * time.Second)
	}
}

func receiver(host string, port int, messages chan string) {
	for {
		err := updateIp()
		if err == nil {
			var address = host + ":" + strconv.Itoa(port)
			log.Printf("Trying to connect receiver to %s", address)
			conn, err := net.Dial("tcp", address)
			if err == nil {
				log.Printf("Connected receiver to %s", address)
				reader := bufio.NewReader(conn)
				for {
					receivedMessage, readErr := reader.ReadString('\n')
					if readErr == nil {
						message := strings.TrimSpace(receivedMessage)
						messages <- message
						if message != "" {
							messagesReceived.Inc()
						}
					} else {
						break
					}
				}
				closeErr := conn.Close()
				if closeErr != nil {
					log.Printf("Could not close connection %v", closeErr)
				}
			} else {
				log.Printf("Connection failed to receiver %s", address)
			}
		} else {
			log.Printf("")
		}
		time.Sleep(10 * time.Second)
	}
}

func prom() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":9180", nil)
	if err != nil {
		log.Printf("Could not start metric endpoint %v", err)
	}
}

func main() {
	sourceHost := flag.String("source-host", "localhost", "Receiver hostname")
	sourcePort := flag.Int("source-port", 30003, "Receiver port")
	destHost := flag.String("dest-host", "data.adsbhub.org", "Destination hostname")
	destPort := flag.Int("dest-port", 5001, "Destination port")
	key := flag.String("client-key", "", "Station dynamic IP update ckey")

	flag.Parse()

	clientKey = *key
	var messages = make(chan string, 100)
	go sender(*destHost, *destPort, messages)
	go prom()
	receiver(*sourceHost, *sourcePort, messages)

}

func updateIp() error {
	ip, err := httpGet("https://www.adsbhub.org/getmyip.php")
	if err == nil {
		if currentIp != ip {
			log.Printf("IP is different current ip: %s old ip: %s", ip, currentIp)

			sessionId := md5sum(clientKey)

			key, err := httpGet("https://www.adsbhub.org/key.php")
			if err == nil {
				md5sum := md5sum(sessionId + key)
				result, err := httpGet("https://www.adsbhub.org/updateip.php?sessid=" + md5sum + key + "&myip=" + ip + "&myip6=::")

				if err == nil {
					if md5sum+key == result {
						log.Printf("Successfully updated ip: %s", ip)
						currentIp = ip
						return nil
					} else {
						return fmt.Errorf("error trying to update ip: %s", ip)
					}
				} else {
					return err
				}
			}
		}
		return nil
	} else {
		return err
	}
}

func md5sum(data string) string {
	md5Bytes := md5.Sum([]byte(data))
	return hex.EncodeToString(md5Bytes[:])
}

func httpGet(url string) (string, error) {
	resp, err := http.Get(url)
	if err == nil {
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		return string(data), err
	} else {
		return "", err
	}
}
