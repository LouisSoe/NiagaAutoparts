package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	repo      *repository.UserRepository
	logger    *zap.Logger
	jwtSecret string
}

func NewUserService(repo *repository.UserRepository, logger *zap.Logger, jwtSecret string) *UserService {
	return &UserService{
		repo:      repo,
		logger:    logger,
		jwtSecret: jwtSecret,
	}
}

type CreateUserInput struct {
	Email    string         `json:"email"`
	Password string         `json:"password"`
	Name     string         `json:"name"`
	Role     model.UserRole `json:"role"`
	Phone    string         `json:"phone"`
}

type UpdateUserInput struct {
	Email    string         `json:"email"`
	Password string         `json:"password,omitempty"` // Optional
	Name     string         `json:"name"`
	Role     model.UserRole `json:"role"`
	Phone    string         `json:"phone"`
	IsActive bool           `json:"is_active"`
}

func (s *UserService) CreateUser(ctx context.Context, input CreateUserInput) (*model.User, error) {
	input.Email = strings.TrimSpace(input.Email)
	if input.Email == "" {
		return nil, fmt.Errorf("email tidak boleh kosong")
	}
	if input.Password == "" {
		return nil, fmt.Errorf("password tidak boleh kosong")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("nama tidak boleh kosong")
	}
	if input.Role == "" {
		input.Role = model.RoleStaff
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	u := &model.User{
		Email:        input.Email,
		PasswordHash: string(hashedPassword),
		Name:         input.Name,
		Role:         input.Role,
		IsActive:     true,
	}
	if input.Phone != "" {
		u.Phone.String = input.Phone
		u.Phone.Valid = true
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) GetAllUsers(ctx context.Context) ([]model.User, error) {
	return s.repo.GetAll(ctx)
}

func (s *UserService) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) UpdateUser(ctx context.Context, id int64, input UpdateUserInput) (*model.User, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Email != "" {
		existing.Email = strings.TrimSpace(input.Email)
	}
	if input.Name != "" {
		existing.Name = strings.TrimSpace(input.Name)
	}
	if input.Role != "" {
		existing.Role = input.Role
	}
	existing.IsActive = input.IsActive

	if input.Phone != "" {
		existing.Phone.String = input.Phone
		existing.Phone.Valid = true
	}

	if input.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		existing.PasswordHash = string(hashedPassword)
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *UserService) GetFilteredUsers(ctx context.Context, filter repository.UserFilter) ([]model.User, int64, error) {
	return s.repo.FindFiltered(ctx, filter)
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResult struct {
	User  *model.User `json:"user"`
	Token string      `json:"token"`
}

func (s *UserService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	email := strings.TrimSpace(input.Email)
	if email == "" {
		return nil, fmt.Errorf("email wajib diisi")
	}
	if strings.TrimSpace(input.Password) == "" {
		return nil, fmt.Errorf("password wajib diisi")
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("kredensial tidak valid")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("akun Anda tidak aktif, silakan hubungi administrator")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, fmt.Errorf("kredensial tidak valid")
	}

	token, err := utils.GenerateJWT(user.ID, user.Email, user.Name, string(user.Role), s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat token autentikasi: %w", err)
	}

	return &LoginResult{
		User:  user,
		Token: token,
	}, nil
}

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
}

func (s *UserService) Register(ctx context.Context, input RegisterInput) (*model.User, error) {
	input.Email = strings.TrimSpace(input.Email)
	input.Name = strings.TrimSpace(input.Name)
	input.Password = strings.TrimSpace(input.Password)
	input.Phone = strings.TrimSpace(input.Phone)

	if input.Email == "" || input.Password == "" || input.Name == "" {
		return nil, fmt.Errorf("semua field wajib diisi")
	}

	existingUser, err := s.repo.GetByEmail(ctx, input.Email)
	if err == nil && existingUser != nil {
		if existingUser.IsActive {
			return nil, fmt.Errorf("email sudah terdaftar")
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("gagal memproses password: %w", err)
		}

		existingUser.Name = input.Name
		existingUser.PasswordHash = string(hashedPassword)
		existingUser.IsActive = true
		if input.Phone != "" {
			existingUser.Phone.String = input.Phone
			existingUser.Phone.Valid = true
		}

		if err := s.repo.Update(ctx, existingUser); err != nil {
			return nil, fmt.Errorf("gagal memperbarui akun: %w", err)
		}
		return existingUser, nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("gagal memproses password: %w", err)
	}

	newUser := &model.User{
		Email:        input.Email,
		PasswordHash: string(hashedPassword),
		Name:         input.Name,
		Role:         model.RoleCustomer,
		IsActive:     true,
	}
	if input.Phone != "" {
		newUser.Phone.String = input.Phone
		newUser.Phone.Valid = true
	}

	if err := s.repo.Create(ctx, newUser); err != nil {
		return nil, fmt.Errorf("gagal mendaftarkan user baru: %w", err)
	}

	return newUser, nil
}
