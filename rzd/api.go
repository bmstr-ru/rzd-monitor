package rzd

import (
	"bmstr.ru/rzd-monitor/monitor"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	urlpkg "net/url"
	"strings"
	"time"
)

type TrainRequest struct {
	Origin              string
	Destination         string
	DepartureDate       string
	TimeFrom            int
	TimeTo              int
	CarGrouping         string
	GetByLocalTime      bool
	SpecialPlacesDemand string
}

type TrainsResponse struct {
	Trains []ScheduledTrain
}

type ScheduledTrain struct {
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

type SuggestionResponse struct {
	City  []Suggestion
	Train []Suggestion
}

type Suggestion struct {
	Name        string
	ExpressCode string
}

type FreeSeats struct {
	Lower int
	Upper int
}

var client *http.Client

var rzdSite = "https://ticket.rzd.ru"
var wiremock = "http://127.0.0.1:8888"
var baseUrl = rzdSite

func init() {
	jar, _ := cookiejar.New(nil)
	client = &http.Client{
		Jar:       jar,
		Transport: &AddHeaderTransport{T: http.DefaultTransport},
	}
}

func FindStation(stationName string) ([]*monitor.Station, error) {
	response, err := suggestStations(stationName)
	if err != nil {
		return nil, err
	}

	suggestions, err := parseSuggestions(response)
	if err != nil {
		return nil, err
	}

	if len(suggestions) == 0 {
		return nil, nil
	}

	if len(suggestions) == 1 {
		return sliceOf(&monitor.Station{
			Name: suggestions[0].Name,
			Id:   suggestions[0].ExpressCode,
		}), nil
	}

	for _, suggestion := range suggestions {
		if strings.EqualFold(stationName, suggestion.Name) {
			return sliceOf(&monitor.Station{
				Name: suggestion.Name,
				Id:   suggestion.ExpressCode,
			}), nil
		}
	}

	stations := []*monitor.Station{}
	for _, suggestion := range suggestions {
		stations = append(stations, &monitor.Station{
			Name: suggestion.Name,
			Id:   suggestion.ExpressCode,
		})
	}
	return stations, nil
}

func sliceOf(station *monitor.Station) []*monitor.Station {
	return []*monitor.Station{station}
}

func FindFreeSeats(trainNumber string, from string, to string, date time.Time) (*FreeSeats, error) {
	response, err := requestTrains(from, to, date)
	if err != nil {
		return nil, err
	}

	trains, err := parseTrains(response)
	if err != nil {
		return nil, err
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
	return &freeSeats, nil
}

func suggestStations(station string) (string, error) {
	url := baseUrl + "/api/v1/suggests" +
		"?GroupResults=true" +
		"&RailwaySortPriority=true" +
		"&MergeSuburban=true" +
		"&Query=" + urlpkg.QueryEscape(station) +
		"&Language=ru" +
		"&TransportType=rail,suburban,bus,aeroexpress"
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	return performRequest(request)
}

func requestTrains(origin string, destination string, date time.Time) (string, error) {
	body := TrainRequest{
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
	request, _ := http.NewRequest("POST", baseUrl+"/apib2b/p/Railway/V1/Search/TrainPricing", bytes.NewBuffer(jsonBody))
	return performRequest(request)
}

func performRequest(req *http.Request) (string, error) {
	response, err := client.Do(req)

	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBytes, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		return "", errors.New("Error Http response: " + response.Status + "\n" + string(responseBytes))
	}
	return string(responseBytes), nil
}

func parseSuggestions(responseStr string) ([]Suggestion, error) {
	var response SuggestionResponse
	err := json.Unmarshal([]byte(responseStr), &response)
	if err != nil {
		return nil, err
	}
	suggestMap := make(map[string]Suggestion)
	for _, suggest := range response.Train {
		suggestMap[suggest.ExpressCode] = suggest
	}
	for _, suggest := range response.City {
		suggestMap[suggest.ExpressCode] = suggest
	}

	suggests := []Suggestion{}
	for _, value := range suggestMap {
		suggests = append(suggests, value)
	}
	return suggests, nil
}

func parseTrains(responseStr string) ([]ScheduledTrain, error) {
	var response TrainsResponse
	err := json.Unmarshal([]byte(responseStr), &response)
	if err != nil {
		return nil, err
	}
	return response.Trains, nil
}
