package model

type Service struct {
	Name       string `json:"name"`
	Deployment *Deployment
}
