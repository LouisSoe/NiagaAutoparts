package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

type CustomerRepository struct {
	db *sqlx.DB
}

func NewCustomerRepository(db *sqlx.DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

func (r *CustomerRepository) Create(ctx context.Context, c *model.Customer) error {
	if c.TypeCustomer == "" {
		c.TypeCustomer = model.CustomerTypeIndividual
	}
	const q = `
		INSERT INTO customers (user_id, type_customer, address, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q, c.UserID, c.TypeCustomer, c.Address, c.Notes).
		Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *CustomerRepository) GetAll(ctx context.Context) ([]model.Customer, error) {
	const q = `
		SELECT c.id, COALESCE(c.user_id, 0) AS user_id, c.type_customer, COALESCE(u.name, '') AS name, COALESCE(u.phone, '') AS phone, COALESCE(u.email, '') AS email, c.address, c.notes, c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN users u ON u.id = c.user_id
		ORDER BY c.id DESC`
	var customers []model.Customer
	err := r.db.SelectContext(ctx, &customers, q)
	return customers, err
}

func (r *CustomerRepository) GetByID(ctx context.Context, id int64) (*model.Customer, error) {
	const q = `
		SELECT c.id, COALESCE(c.user_id, 0) AS user_id, c.type_customer, COALESCE(u.name, '') AS name, COALESCE(u.phone, '') AS phone, COALESCE(u.email, '') AS email, c.address, c.notes, c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.id = $1`
	var c model.Customer
	if err := r.db.GetContext(ctx, &c, q, id); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CustomerRepository) GetByUserID(ctx context.Context, userID int64) (*model.Customer, error) {
	const q = `
		SELECT c.id, COALESCE(c.user_id, 0) AS user_id, c.type_customer, COALESCE(u.name, '') AS name, COALESCE(u.phone, '') AS phone, COALESCE(u.email, '') AS email, c.address, c.notes, c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.user_id = $1`
	var c model.Customer
	if err := r.db.GetContext(ctx, &c, q, userID); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CustomerRepository) GetUserBasicInfo(ctx context.Context, userID int64) (*model.Customer, error) {
	const q = `
		SELECT 0 AS id, id AS user_id, 'individual' AS type_customer, name, COALESCE(phone, '') AS phone, email, '' AS address, '' AS notes, created_at, updated_at
		FROM users
		WHERE id = $1`
	var c model.Customer
	if err := r.db.GetContext(ctx, &c, q, userID); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CustomerRepository) UpdateUserInfo(ctx context.Context, userID int64, name, phone string) error {
	const q = `UPDATE users SET name = COALESCE(NULLIF($1, ''), name), phone = COALESCE(NULLIF($2, ''), phone), updated_at = NOW() WHERE id = $3`
	_, err := r.db.ExecContext(ctx, q, name, phone, userID)
	return err
}

func (r *CustomerRepository) GetByPhone(ctx context.Context, phone string) (*model.Customer, error) {
	const q = `
		SELECT c.id, COALESCE(c.user_id, 0) AS user_id, c.type_customer, COALESCE(u.name, '') AS name, COALESCE(u.phone, '') AS phone, COALESCE(u.email, '') AS email, c.address, c.notes, c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE u.phone = $1`
	var c model.Customer
	if err := r.db.GetContext(ctx, &c, q, phone); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CustomerRepository) Update(ctx context.Context, c *model.Customer) error {
	const q = `
		UPDATE customers
		SET user_id = $1, type_customer = $2, address = $3, notes = $4, updated_at = NOW()
		WHERE id = $5`
	res, err := r.db.ExecContext(ctx, q, c.UserID, c.TypeCustomer, c.Address, c.Notes, c.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("customer not found")
	}
	return nil
}

func (r *CustomerRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM customers WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("customer not found")
	}
	return nil
}

type CustomerFilter struct {
	Q     string
	Type  string
	Page  int
	Limit int
}

func (r *CustomerRepository) FindFiltered(ctx context.Context, filter CustomerFilter) ([]model.Customer, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filter.Q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(COALESCE(u.name, '') ILIKE $%d OR COALESCE(u.phone, '') ILIKE $%d OR COALESCE(u.email, '') ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+filter.Q+"%")
		argIdx++
	}

	if filter.Type != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("UPPER(c.type_customer) = $%d", argIdx))
		args = append(args, strings.ToUpper(filter.Type))
		argIdx++
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM customers c 
		LEFT JOIN users u ON u.id = c.user_id 
		WHERE %s`, whereStmt)

	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered count: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT c.id, COALESCE(c.user_id, 0) AS user_id, c.type_customer, COALESCE(u.name, '') AS name, COALESCE(u.phone, '') AS phone, COALESCE(u.email, '') AS email, c.address, c.notes, c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE %s
		ORDER BY c.id DESC`, whereStmt)

	if filter.Limit > 0 {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		offset := (filter.Page - 1) * filter.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, filter.Limit, offset)
	}

	var customers []model.Customer
	if err := r.db.SelectContext(ctx, &customers, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered select: %w", err)
	}

	return customers, total, nil
}
