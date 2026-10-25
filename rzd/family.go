package rzd

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

type FamilyStatus string

const (
	FamilyUnavailable FamilyStatus = "unavailable"
	FamilyAvailable   FamilyStatus = "available"
)

// FamilyAvailability uses EmptyCabinQuantity for ordinary compartment availability.
// EmptyCabinQuantitySum preserves the raw API units, not a cabin count.
type FamilyAvailability struct {
	Status                FamilyStatus
	Lower                 int
	Upper                 int
	Total                 int
	EmptyCabinQuantitySum int
	Groups                []CarGroup
}

func FindFamilyAvailability(trainNumber, from, to string, date time.Time) (*FamilyAvailability, error) {
	response, err := requestTrains(from, to, date)
	if err != nil {
		return nil, err
	}
	trains, err := parseTrains(response)
	if err != nil {
		return nil, err
	}
	return evaluateFamilyAvailability(trains, trainNumber)
}

func matchesTrain(actual, requested string) bool {
	actual = strings.ToUpper(strings.TrimSpace(actual))
	requested = strings.ToUpper(strings.TrimSpace(requested))
	if requested == "" {
		return false
	}
	if actual == requested {
		return true
	}
	// A number entered without a letter may match the train's full designation.
	return strings.IndexFunc(requested, unicode.IsLetter) == -1 && strings.TrimRightFunc(actual, unicode.IsLetter) == requested
}

func evaluateFamilyAvailability(trains []ScheduledTrain, number string) (*FamilyAvailability, error) {
	result := &FamilyAvailability{Status: FamilyUnavailable}
	found := false
	for _, train := range trains {
		if !matchesTrain(train.TrainNumber, number) {
			continue
		}
		if found {
			return nil, fmt.Errorf("train number %q is ambiguous; enter the full number with its letter", number)
		}
		found = true
		if train.IsSaleForbidden {
			continue
		}
		for _, group := range train.CarGroups {
			// This is deliberately conservative: a group marked as containing disabled
			// places is excluded as a whole until seat-level restrictions are available.
			if group.CarType != "Compartment" || group.IsSaleForbidden || group.HasPlacesForDisabledPersons {
				continue
			}
			result.Groups = append(result.Groups, group)
			result.Lower += group.LowerPlaceQuantity
			result.Upper += group.UpperPlaceQuantity
			result.Total += group.TotalPlaceQuantity
			if group.EmptyCabinQuantity > 0 {
				result.EmptyCabinQuantitySum += group.EmptyCabinQuantity
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("train %q not found in TrainPricing response", number)
	}
	// EmptyCabinQuantity is the availability criterion; scattered free seats
	// do not qualify, regardless of their total or lower/upper counts.
	if result.EmptyCabinQuantitySum > 0 {
		result.Status = FamilyAvailable
	}

	return result, nil
}
