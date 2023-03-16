package api

import (
	"bmstr.ru/rzd-monitor/monitor"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type Request struct {
	Origin              string
	Destination         string
	DepartureDate       string
	TimeFrom            int
	TimeTo              int
	CarGrouping         string
	GetByLocalTime      bool
	SpecialPlacesDemand string
}

type Response struct {
	Trains []Train
}

type Train struct {
	CarGroups   []CarGroup
	TrainNumber string
}

type CarGroup struct {
	LowerPlaceQuantity          int
	UpperPlaceQuantity          int
	LowerSidePlaceQuantity      int
	UpperSidePlaceQuantity      int
	IsSaleForbidden             bool
	HasPlacesForDisabledPersons bool
}

type FreeSeats struct {
	Lower int
	Upper int
}

func FindStation(stationName string) *monitor.Station {
	switch strings.ToLower(stationName) {
	case "кемь":
		return &monitor.Station{Name: "Кемь", Id: 2004720}
	case "санкт-петербург":
		return &monitor.Station{Name: "Санкт-Петербург", Id: 2004000}
	}
	return nil
}

func FindFreeSeats(trainNumber string, from string, to string, date time.Time) (FreeSeats, error) {
	response, err := fetch(from, to, date)
	if err != nil {
		return FreeSeats{}, err
	}

	trains, err := parseTrains(response)
	if err != nil {
		return FreeSeats{}, err
	}

	freeSeats := FreeSeats{}
	for i := 0; i < len(trains); i++ {
		train := trains[i]
		if strings.HasPrefix(train.TrainNumber, trainNumber) {
			for j := 0; j < len(train.CarGroups); j++ {
				carGroup := train.CarGroups[j]
				if !carGroup.HasPlacesForDisabledPersons && !carGroup.IsSaleForbidden {
					freeSeats.Lower += carGroup.LowerPlaceQuantity
					freeSeats.Lower += carGroup.LowerSidePlaceQuantity
					freeSeats.Upper += carGroup.UpperPlaceQuantity
					freeSeats.Upper += carGroup.UpperSidePlaceQuantity
				}
			}
		}
	}
	return freeSeats, nil
}

func fetch(origin string, destination string, date time.Time) (string, error) {
	client := http.DefaultClient
	body := Request{
		Origin:              origin,
		Destination:         destination,
		DepartureDate:       date.Format("2006-01-02T15:04:05"),
		TimeFrom:            0,
		TimeTo:              24,
		CarGrouping:         "DontGroup",
		GetByLocalTime:      true,
		SpecialPlacesDemand: "StandardPlacesAndForDisabledPersons",
	}
	jsonBody, _ := json.Marshal(body)
	request, _ := http.NewRequest("POST", "https://ticket.rzd.ru/apib2b/p/Railway/V1/Search/TrainPricing", bytes.NewBuffer(jsonBody))
	request.Header = http.Header{
		"Content-Type":    {"application/json"},
		"Pragma":          {"no-cache"},
		"Accept":          {"application/json, text/plain, */*"},
		"Accept-Language": {"en-GB,en;q=0.9"},
		"Accept-Encoding": {"deflate"},
		"Cache-Control":   {"no-cache"},
		"Origin":          {"https://ticket.rzd.ru"},
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15"},
		"Referer":         {"https://ticket.rzd.ru/searchresults/v/1/5a13ba4b340c745ca1e7dda0/5a3244bc340c7441a0a556ca/2023-04-02"},
		"Cookie":          {"_ga=GA1.1.118455456.1678553833; _ga_J932X5NKG9=GS1.1.1678563767.3.0.1678563767.0.0.0; _ym_visorc=b; LANG_SITE=ru; _gid=GA1.2.409377037.1678553833; accessible=false; _ym_isad=2; session-cookie=174b6bd11685cf21046c7a5be59e4e026eb27a5669e9bdf8cd4d5e1aca896abf44a23f51c2dee2da12b5a6e73db142e4; lang=ru; acceptCookies=1; jsonFormat=true; _ym_d=1678212254; _ym_uid=1678212254580970909"},
		"sentry-trace":    {"5ba9af2e58b042a19c40ea02a4ca554f-abd925811d5d2cb0-1"},
	}

	response, err := client.Do(request)

	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", errors.New("Error Http response: " + response.Status)
	}
	responseBytes, _ := io.ReadAll(response.Body)
	return string(responseBytes), nil
}

func parseTrains(responseStr string) ([]Train, error) {
	var response Response
	err := json.Unmarshal([]byte(responseStr), &response)
	if err != nil {
		return nil, err
	}
	return response.Trains, nil
}
