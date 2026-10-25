package rzd

import (
	"testing"
)

func TestFamilyAvailability(t *testing.T) {
	tests := []struct {
		name, body          string
		total, lower, upper int
		status              FamilyStatus
	}{
		{"015A supplied aggregates", `{"Trains":[{"TrainNumber":"015А","CarGroups":[
   {"CarType":"Luxury","TotalPlaceQuantity":1,"UpperPlaceQuantity":1},
   {"CarType":"Compartment","TotalPlaceQuantity":4,"UpperPlaceQuantity":4,"EmptyCabinQuantity":0},
   {"CarType":"ReservedSeat","TotalPlaceQuantity":15,"UpperPlaceQuantity":8,"UpperSidePlaceQuantity":7},
   {"CarType":"ReservedSeat","TotalPlaceQuantity":36,"LowerPlaceQuantity":1,"UpperPlaceQuantity":22,"UpperSidePlaceQuantity":13}
  ]}]}`, 4, 0, 4, FamilyUnavailable},
		{"011A supplied aggregates", `{"Trains":[{"TrainNumber":"011А","CarGroups":[
   {"CarType":"Compartment","TotalPlaceQuantity":2,"UpperPlaceQuantity":2},
   {"CarType":"Compartment","TotalPlaceQuantity":1,"UpperPlaceQuantity":1},
   {"CarType":"Compartment","TotalPlaceQuantity":1,"UpperPlaceQuantity":1},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"UpperPlaceQuantity":2},
   {"CarType":"Compartment","TotalPlaceQuantity":1,"UpperPlaceQuantity":1},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"LowerPlaceQuantity":1,"UpperPlaceQuantity":1,"HasPlacesForDisabledPersons":true,"EmptyCabinQuantity":1},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"UpperPlaceQuantity":2},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"UpperPlaceQuantity":2},
   {"CarType":"Compartment","TotalPlaceQuantity":3,"UpperPlaceQuantity":3}
  ]}]}`, 14, 0, 14, FamilyUnavailable},
		{"four scattered places do not qualify", `{"Trains":[{"TrainNumber":"015А","CarGroups":[{"CarType":"Compartment","TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2,"EmptyCabinQuantity":0}]}]}`, 4, 2, 2, FamilyUnavailable},
		{"empty cabin triggers availability", `{"Trains":[{"TrainNumber":"015А","CarGroups":[{"CarType":"Compartment","TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2,"EmptyCabinQuantity":1}]}]}`, 4, 2, 2, FamilyAvailable},
		{"disabled and forbidden excluded", `{"Trains":[{"TrainNumber":"015А","CarGroups":[{"CarType":"Compartment","TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2,"HasPlacesForDisabledPersons":true,"EmptyCabinQuantity":1},{"CarType":"Compartment","TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2,"IsSaleForbidden":true,"EmptyCabinQuantity":1}]}]}`, 0, 0, 0, FamilyUnavailable},
		{"train sale forbidden", `{"Trains":[{"TrainNumber":"015А","IsSaleForbidden":true,"CarGroups":[{"CarType":"Compartment","TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2}]}]}`, 0, 0, 0, FamilyUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trains, err := parseTrains(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			result, err := evaluateFamilyAvailability(trains, trains[0].TrainNumber)
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != tc.total || result.Lower != tc.lower || result.Upper != tc.upper || result.Status != tc.status {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}
func TestTrainMatching(t *testing.T) {
	if matchesTrain("015А", "01") || matchesTrain("015А", "") || matchesTrain("015А", "015Б") {
		t.Fatal("incorrect partial match")
	}
	if !matchesTrain("015А", "015") || !matchesTrain("015А", "015а") {
		t.Fatal("train must match")
	}
	if _, err := evaluateFamilyAvailability(nil, "015"); err == nil {
		t.Fatal("missing train must be an error")
	}
	if _, err := evaluateFamilyAvailability([]ScheduledTrain{{TrainNumber: "015А"}, {TrainNumber: "015Б"}}, "015"); err == nil {
		t.Fatal("ambiguous train must be an error")
	}
}

func TestEmptyCabinCountExcludesIneligibleGroups(t *testing.T) {
	result, err := evaluateFamilyAvailability([]ScheduledTrain{{TrainNumber: "015А", CarGroups: []CarGroup{
		{CarType: "Compartment", EmptyCabinQuantity: 2},
		{CarType: "Compartment", EmptyCabinQuantity: 1},
		{CarType: "Compartment", EmptyCabinQuantity: 4, HasPlacesForDisabledPersons: true},
		{CarType: "Compartment", EmptyCabinQuantity: 5, IsSaleForbidden: true},
		{CarType: "Luxury", EmptyCabinQuantity: 6},
		{CarType: "ReservedSeat", EmptyCabinQuantity: 7},
	}}}, "015А")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != FamilyAvailable || result.EmptyCabinQuantitySum != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// Relevant fields from the supplied 2027-01-20 response. EmptyCabinQuantity
// appears to count places, so preserve its raw value without calling it cabins.
func TestJanuary20EmptyCabinResponse(t *testing.T) {
	trains, err := parseTrains(`{"Trains":[
  {"TrainNumber":"015А","CarGroups":[
   {"CarType":"Luxury","TotalPlaceQuantity":12,"LowerPlaceQuantity":6,"UpperPlaceQuantity":6,"EmptyCabinQuantity":6},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"LowerPlaceQuantity":1,"UpperPlaceQuantity":1,"HasPlacesForDisabledPersons":true},
   {"CarType":"Compartment","ServiceClasses":["2Т"],"TotalPlaceQuantity":4,"LowerPlaceQuantity":2,"UpperPlaceQuantity":2,"EmptyCabinQuantity":4},
   {"CarType":"Compartment","ServiceClasses":["2Т"],"TotalPlaceQuantity":24,"LowerPlaceQuantity":12,"UpperPlaceQuantity":12,"EmptyCabinQuantity":24}
  ]},
  {"TrainNumber":"011А","CarGroups":[
   {"CarType":"Compartment","TotalPlaceQuantity":106,"LowerPlaceQuantity":52,"UpperPlaceQuantity":54,"EmptyCabinQuantity":0},
   {"CarType":"Compartment","ServiceClasses":["2Х"],"TotalPlaceQuantity":8,"LowerPlaceQuantity":4,"UpperPlaceQuantity":4,"EmptyCabinQuantity":8},
   {"CarType":"Compartment","ServiceClasses":["2Х"],"TotalPlaceQuantity":55,"LowerPlaceQuantity":28,"UpperPlaceQuantity":27,"EmptyCabinQuantity":52},
   {"CarType":"Compartment","TotalPlaceQuantity":2,"LowerPlaceQuantity":1,"UpperPlaceQuantity":1,"HasPlacesForDisabledPersons":true}
  ]}
 ]}`)
	if err != nil {
		t.Fatal(err)
	}
	for number, want := range map[string]int{"015А": 28, "011А": 60} {
		result, err := evaluateFamilyAvailability(trains, number)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != FamilyAvailable || result.EmptyCabinQuantitySum != want {
			t.Fatalf("%s: unexpected result: %+v", number, result)
		}
	}
}
