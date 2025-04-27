package service

import (
	"errors"

	"gitlab.ozon.dev/gojhw1/pkg/model"
)

var (
	ErrUnknownWrapperType = errors.New("неизвестный тип обертки")
)

type wrapperDecorator struct {
	packager    packager
	description string
	cost        float64
}

// newWrapperDecorator создает новый экземпляр wrapperDecorator
func newWrapperDecorator(packager packager, wrapperType model.WrapperType) (*wrapperDecorator, error) {
	switch wrapperType {
	case model.WrapperFilm:
		return &wrapperDecorator{
			packager:    packager,
			description: "film",
			cost:        filmCost,
		}, nil
	default:
		return nil, ErrUnknownWrapperType
	}
}

func (d *wrapperDecorator) validateWeight(weight float64) error {
	return d.packager.validateWeight(weight)
}

func (d *wrapperDecorator) getAdditionalCost() float64 {
	return d.packager.getAdditionalCost() + d.cost
}

func (d *wrapperDecorator) getDescription() string {
	return d.packager.getDescription() + " + " + d.description
}
