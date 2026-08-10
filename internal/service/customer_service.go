package service

import (
	"context"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

type CustomerService struct {
	repo   *repository.CustomerRepository
	logger *zap.Logger
}

func NewCustomerService(repo *repository.CustomerRepository, logger *zap.Logger) *CustomerService {
	return &CustomerService{
		repo:   repo,
		logger: logger,
	}
}

type CreateCustomerInput struct {
	UserID       int64              `json:"user_id"`
	TypeCustomer model.CustomerType `json:"type_customer"`
	Address      string             `json:"address"`
	Notes        string             `json:"notes"`
}

type UpdateCustomerInput struct {
	UserID       int64              `json:"user_id"`
	TypeCustomer model.CustomerType `json:"type_customer"`
	Address      string             `json:"address"`
	Notes        string             `json:"notes"`
}

func (s *CustomerService) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*model.Customer, error) {
	if input.TypeCustomer == "" {
		input.TypeCustomer = model.CustomerTypeIndividual
	}
	c := &model.Customer{
		UserID:       input.UserID,
		TypeCustomer: input.TypeCustomer,
	}
	if input.Address != "" {
		c.Address.String = input.Address
		c.Address.Valid = true
	}
	if input.Notes != "" {
		c.Notes.String = input.Notes
		c.Notes.Valid = true
	}

	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CustomerService) GetAllCustomers(ctx context.Context) ([]model.Customer, error) {
	return s.repo.GetAll(ctx)
}

func (s *CustomerService) GetCustomerByID(ctx context.Context, id int64) (*model.Customer, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *CustomerService) GetCustomerByUserID(ctx context.Context, userID int64) (*model.Customer, error) {
	c, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return s.repo.GetUserBasicInfo(ctx, userID)
	}
	return c, nil
}

type UpsertProfileInput struct {
	Name         string             `json:"name"`
	Phone        string             `json:"phone"`
	Address      string             `json:"address"`
	TypeCustomer model.CustomerType `json:"type_customer"`
	Notes        string             `json:"notes"`
}

func (s *CustomerService) UpsertProfileByUserID(ctx context.Context, userID int64, input UpsertProfileInput) (*model.Customer, error) {
	if input.Name != "" || input.Phone != "" {
		if err := s.repo.UpdateUserInfo(ctx, userID, input.Name, input.Phone); err != nil {
			s.logger.Warn("failed to update user info during profile upsert", zap.Error(err))
		}
	}

	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if input.TypeCustomer == "" {
			input.TypeCustomer = model.CustomerTypeIndividual
		}
		c := &model.Customer{
			UserID:       userID,
			TypeCustomer: input.TypeCustomer,
		}
		if input.Address != "" {
			c.Address.String = input.Address
			c.Address.Valid = true
		}
		if input.Notes != "" {
			c.Notes.String = input.Notes
			c.Notes.Valid = true
		}
		if err := s.repo.Create(ctx, c); err != nil {
			return nil, err
		}
		return s.repo.GetByUserID(ctx, userID)
	}

	if input.TypeCustomer != "" {
		existing.TypeCustomer = input.TypeCustomer
	}
	if input.Address != "" {
		existing.Address.String = input.Address
		existing.Address.Valid = true
	}
	if input.Notes != "" {
		existing.Notes.String = input.Notes
		existing.Notes.Valid = true
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return s.repo.GetByUserID(ctx, userID)
}

func (s *CustomerService) UpdateCustomer(ctx context.Context, id int64, input UpdateCustomerInput) (*model.Customer, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.UserID > 0 {
		existing.UserID = input.UserID
	}
	if input.TypeCustomer != "" {
		existing.TypeCustomer = input.TypeCustomer
	}
	if input.Address != "" {
		existing.Address.String = input.Address
		existing.Address.Valid = true
	}
	if input.Notes != "" {
		existing.Notes.String = input.Notes
		existing.Notes.Valid = true
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *CustomerService) DeleteCustomer(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *CustomerService) GetFilteredCustomers(ctx context.Context, filter repository.CustomerFilter) ([]model.Customer, int64, error) {
	return s.repo.FindFiltered(ctx, filter)
}
