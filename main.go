package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MichaelS11/go-dht"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tarm/serial"
)

const (
	addr    = ":8081"
	pin     = "GPIO17"
	serial0 = "/dev/serial0"
)

type Temp struct {
	Temp     float64
	Humidity float64
}

func main() {
	mainRoomTemp := prometheus.NewGauge(prometheus.GaugeOpts{Name: "main_room_temp"})
	mainRoomHumidity := prometheus.NewGauge(prometheus.GaugeOpts{Name: "main_room_humidity"})
	mainRoomCO2 := prometheus.NewGauge(prometheus.GaugeOpts{Name: "main_room_co2"})
	prometheus.MustRegister(mainRoomTemp)
	prometheus.MustRegister(mainRoomHumidity)
	prometheus.MustRegister(mainRoomCO2)

	stop := make(chan struct{})
	stopped := make(chan struct{})
	var humidity float64
	var temperature float64

	err := dht.HostInit()
	if err != nil {
		panic(err)
	}
	dht22, err := dht.NewDHT(pin, dht.Celsius, "")
	if err != nil {
		fmt.Println("NewDHT error:", err)
		return
	}

	go dht22.ReadBackground(&humidity, &temperature, time.Second*5, stop, stopped)
	go func() {
		for {
			<-time.After(time.Second * 5)
			mainRoomTemp.Set(temperature)
			mainRoomHumidity.Set(humidity)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/temp", func(writer http.ResponseWriter, request *http.Request) {
		js, _ := json.Marshal(Temp{
			Temp:     temperature,
			Humidity: humidity,
		})
		writer.Write(js)
		writer.WriteHeader(200)
	})
	go startMhz19b(mainRoomCO2, stop)
	go func() {
		err = http.ListenAndServe(addr, nil)
		if err != nil {
			panic(err)
		}
	}()

	fmt.Printf("server started on %s\n", addr)

	term := make(chan os.Signal)
	signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)
	<-term
	fmt.Println("stopping...")
	close(stop)
	<-stopped
	fmt.Println("successfully stopped =)")
}

func startMhz19b(metric prometheus.Gauge, stop chan struct{}) {
	c := &serial.Config{Name: serial0, Baud: 9600, Size: 8, Parity: 0, StopBits: 1}
	port, err := serial.OpenPort(c)
	if err != nil {
		panic(err)
	}
	defer port.Close()
	for {
		select {
		case <-stop:
			return
		case <-time.After(time.Second):
			break
		}
		_, err = port.Write([]byte{0xFF, 0x01, 0x86, 0x00, 0x00, 0x00, 0x00, 0x00, 0x79})
		if err != nil {
			panic(err)
		}
		buf := make([]byte, 9)
		_, err = port.Read(buf)
		if err != nil {
			fmt.Println(err)
			continue
		}
		result := int(buf[2])*256 + int(buf[3])
		metric.Set(float64(result))
		fmt.Println(result)
	}
}
