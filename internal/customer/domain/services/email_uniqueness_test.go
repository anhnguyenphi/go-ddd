package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/mocks"
	"github.com/example/myapp/internal/customer/domain/services"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

func TestEnsureAvailable(t *testing.T) {
	email, _ := domain.NewEmail("a@example.com")
	repoErr := errors.New("db down")

	cases := []struct {
		name   string
		exists bool
		err    error
		want   error
	}{
		{"free", false, nil, nil},
		{"taken", true, nil, shareddomain.ErrConflict},
		{"repository error propagates", false, repoErr, repoErr},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := mocks.NewRepository(t)
			repo.EXPECT().ExistsByEmail(mock.Anything, email).Return(c.exists, c.err).Once()

			err := services.NewEmailUniqueness(repo).EnsureAvailable(context.Background(), email)
			if !errors.Is(err, c.want) || (c.want == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
