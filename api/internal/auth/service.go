package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrEmailTaken         = errors.New("el email ya está registrado")
)

type Service struct {
	q      *store.Queries
	tokens *TokenManager
}

func NewService(q *store.Queries, tm *TokenManager) *Service {
	return &Service{q: q, tokens: tm}
}

func (s *Service) Register(ctx context.Context, email, password, name string) (store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := s.q.GetUserByEmail(ctx, email); err == nil {
		return store.User{}, ErrEmailTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	return s.q.CreateUser(ctx, store.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		Name:         name,
	})
}

func (s *Service) Login(ctx context.Context, email, password string) (store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return store.User{}, ErrInvalidCredentials
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return store.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) IssueTokens(id uuid.UUID) (access, refresh string, err error) {
	access, err = s.tokens.IssueAccess(id)
	if err != nil {
		return "", "", err
	}
	refresh, err = s.tokens.IssueRefresh(id)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (s *Service) Tokens() *TokenManager { return s.tokens }
