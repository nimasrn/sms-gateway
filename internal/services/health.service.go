package services

type HealthService struct {
}

func NewHealthService() HealthService {
	return HealthService{}
}

func (m HealthService) Get() error {
	return nil
}
