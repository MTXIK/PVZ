package service

import (
	"errors"
)

var (
	// ErrPackageWeightExceeded - ошибка, если вес упаковки превышает допустимый
	ErrPackageWeightExceeded = errors.New("превышен максимальный вес для данного типа упаковки")
)

const (
	bagCost  = 5.0
	boxCost  = 20.0
	filmCost = 1.0
)

const (
	bagMaxWeight = 10.0
	boxMaxWeight = 30.0
)

type packager interface {
	validateWeight(weight float64) error
	getAdditionalCost() float64
	getDescription() string
}

type basicPackager struct {
	description string
	maxWeight   float64
	cost        float64
}

func (p *basicPackager) validateWeight(weight float64) error {
	if p.maxWeight > 0 && weight > p.maxWeight {
		return ErrPackageWeightExceeded
	}
	return nil
}

func (p *basicPackager) getAdditionalCost() float64 {
	return p.cost
}

func (p *basicPackager) getDescription() string {
	return p.description
}

type bagPackager struct {
	basicPackager
}

func newBagPackager() *bagPackager {
	return &bagPackager{
		basicPackager{
			description: "bag",
			maxWeight:   bagMaxWeight,
			cost:        bagCost,
		},
	}
}

type boxPackager struct {
	basicPackager
}

func newBoxPackager() *boxPackager {
	return &boxPackager{
		basicPackager{
			description: "box",
			maxWeight:   boxMaxWeight,
			cost:        boxCost,
		},
	}
}

type FilmPackager struct {
	basicPackager
}

func newFilmPackager() *FilmPackager {
	return &FilmPackager{
		basicPackager{
			description: "film",
			cost:        filmCost,
		},
	}
}
