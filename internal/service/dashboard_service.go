package service

import (
	"context"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

type DashboardService struct {
	repo   *repository.DashboardRepository
	logger *zap.Logger
}

func NewDashboardService(repo *repository.DashboardRepository, logger *zap.Logger) *DashboardService {
	return &DashboardService{
		repo:   repo,
		logger: logger,
	}
}

func (s *DashboardService) GetDashboardData(ctx context.Context) (*model.DashboardData, error) {
	summary, err := s.repo.GetSummary(ctx)
	if err != nil {
		s.logger.Error("failed to fetch dashboard summary", zap.Error(err))
		return nil, err
	}

	recentOrders, err := s.repo.GetRecentOrders(ctx, 5)
	if err != nil {
		s.logger.Error("failed to fetch dashboard recent orders", zap.Error(err))
		return nil, err
	}

	lowStockItems, err := s.repo.GetLowStockItems(ctx, 10)
	if err != nil {
		s.logger.Error("failed to fetch dashboard low stock items", zap.Error(err))
		return nil, err
	}

	return &model.DashboardData{
		Summary:       *summary,
		RecentOrders:  recentOrders,
		LowStockItems: lowStockItems,
	}, nil
}
