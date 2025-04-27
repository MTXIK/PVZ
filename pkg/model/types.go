package model

type OrderState string

const (
	StateAccepted  OrderState = "accepted"
	StateDelivered OrderState = "delivered"
	StateReturned  OrderState = "returned"
)

type PackageType string

const (
	PackageBag  PackageType = "bag"
	PackageBox  PackageType = "box"
	PackageFilm PackageType = "film"
)

type WrapperType string

const (
	WrapperFilm WrapperType = "film"
)
