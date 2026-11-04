package main

import "time"

func duration(start, end string) (float64, error) {
	s, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return 0, err
	}
	e, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return 0, err
	}
	return e.Sub(s).Seconds(), nil
}
