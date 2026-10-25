package rzd

import (
	"bmstr.ru/rzd-monitor/logging"
	"bmstr.ru/rzd-monitor/monitor"
	"bytes"
	"encoding/json"
	"fmt"
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
	CarGroups       []CarGroup
	TrainNumber     string
	IsSaleForbidden bool
}

type CarGroup struct {
	CarType                     string
	ServiceClasses              []string
	TotalPlaceQuantity          int
	EmptyCabinQuantity          int
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

var client *http.Client

var rzdSite = "https://ticket.rzd.ru"
var wiremock = "http://127.0.0.1:8888"
var baseUrl = rzdSite

func init() {
	jar, _ := cookiejar.New(nil)
	client = &http.Client{
		Jar:       jar,
		Transport: &AddHeaderTransport{T: &logging.Transport{Service: "rzd"}},
		Timeout:   30 * time.Second,
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
		return "", err
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
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read RZD response: %w", err)
	}
	if response.StatusCode != 200 {
		return "", fmt.Errorf("RZD HTTP %s", response.Status)
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
