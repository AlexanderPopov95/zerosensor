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
)

const (
	addr = ":8081"
	pin  = "GPIO17"
)

type Temp struct {
	Temp     float64
	Humidity float64
}

func main() {
	mainRoomTemp := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "main_room_temp",
	})
	mainRoomHumidity := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "main_room_humidity",
	})

	prometheus.MustRegister(mainRoomTemp)
	prometheus.MustRegister(mainRoomHumidity)

	err := dht.HostInit()
	if err != nil {
		panic(err)
	}

	dht22, err := dht.NewDHT(pin, dht.Celsius, "")
	if err != nil {
		fmt.Println("NewDHT error:", err)
		return
	}

	stop := make(chan struct{})
	stopped := make(chan struct{})
	var humidity float64
	var temperature float64

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
