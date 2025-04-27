package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/service"
)

func TestParseDeadline(t *testing.T) {
	t.Parallel()

	timeStr := time.Now().Add(24 * time.Hour).Format(timeLayout)

	tests := []struct {
		name        string
		deadlineStr string
		wantErr     bool
	}{
		{
			name:        "valid deadline duration",
			deadlineStr: "24h",
			wantErr:     false,
		},
		{
			name:        "valid date",
			deadlineStr: timeStr,
			wantErr:     false,
		},
		{
			name:        "invalid date",
			deadlineStr: "invalid",
			wantErr:     true,
		},
		{
			name:        "empty string",
			deadlineStr: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseDeadline(tt.deadlineStr)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, result.IsZero())
			}
		})
	}
}

func TestProcessError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "Bad request",
			err:        service.ErrOpenFile,
			wantStatus: 400,
		},
		{
			name:       "Conflict",
			err:        service.ErrOrderExists,
			wantStatus: 409,
		},
		{
			name:       "Forbidden",
			err:        service.ErrWrongCustomer,
			wantStatus: 403,
		},
		{
			name:       "Gone",
			err:        service.ErrStorageExpired,
			wantStatus: 410,
		},
		{
			name:       "Not found",
			err:        repository.ErrOrderNotFound,
			wantStatus: 404,
		},
		{
			name:       "Default",
			err:        errors.New("Default"),
			wantStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := processError(tt.err)
			assert.Equal(t, tt.wantStatus, got)
			assert.Equal(t, tt.err.Error(), err)
		})
	}
}

func TestParseCursorFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cursorStr string
		want      int64
		wantErr   bool
	}{
		{
			name:      "Correct cursor",
			cursorStr: "123",
			want:      123,
			wantErr:   false,
		},
		{
			name:      "Empty cursor",
			cursorStr: "",
			want:      0,
			wantErr:   false,
		},
		{
			name:      "Negative cursor",
			cursorStr: "-1",
			want:      0,
			wantErr:   true,
		},
		{
			name:      "Invalid cursor",
			cursorStr: "invalid",
			want:      0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseCursorFromString(tt.cursorStr)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestParseLimitFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		limitStr    string
		defaultSize int
		want        int
		wantErr     bool
	}{
		{
			name:        "Correct limit",
			limitStr:    "10",
			defaultSize: 5,
			want:        10,
			wantErr:     false,
		},
		{
			name:        "Empty limit",
			limitStr:    "",
			defaultSize: 5,
			want:        5,
			wantErr:     false,
		},
		{
			name:        "Invalid limit",
			limitStr:    "invalid",
			defaultSize: 5,
			want:        0,
			wantErr:     true,
		},
		{
			name:        "Negative limit",
			limitStr:    "-1",
			defaultSize: 5,
			want:        0,
			wantErr:     true,
		},
		{
			name:        "Too big limit",
			limitStr:    "101",
			defaultSize: 5,
			want:        0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseLimitFromString(tt.limitStr, tt.defaultSize)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestParseCustomerIDFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		customerIDStr string
		want          int64
		wantErr       bool
	}{
		{
			name:          "Correct customer ID",
			customerIDStr: "123",
			want:          123,
			wantErr:       false,
		},
		{
			name:          "Empty customer ID",
			customerIDStr: "",
			want:          0,
			wantErr:       true,
		},
		{
			name:          "Negative customer ID",
			customerIDStr: "-1",
			want:          0,
			wantErr:       true,
		},
		{
			name:          "Invalid customer ID",
			customerIDStr: "invalid",
			want:          0,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseCustomerIDFromString(tt.customerIDStr)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestValidateOrderRequest(t *testing.T) {
	t.Parallel()

	validTime := time.Now().Add(time.Hour * 24)
	packageBox := model.PackageBox
	wrapperFilm := model.WrapperFilm

	tests := []struct {
		name             string
		req              orderRequest
		wantErr          error
		expectedDeadline time.Time
		expectedPackage  *model.PackageType
		expectedWrapper  *model.WrapperType
	}{
		{
			name: "Valid request with all fields",
			req: orderRequest{
				Weight:      1.5,
				Cost:        1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
				Wrapper:     string(wrapperFilm),
			},
			wantErr:          nil,
			expectedDeadline: validTime,
			expectedPackage:  &packageBox,
			expectedWrapper:  &wrapperFilm,
		},
		{
			name: "Valid request only package type",
			req: orderRequest{
				Weight:      1.5,
				Cost:        1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
				Wrapper:     "",
			},
			wantErr:          nil,
			expectedDeadline: validTime,
			expectedPackage:  &packageBox,
			expectedWrapper:  nil,
		},
		{
			name: "Valid request without package type",
			req: orderRequest{
				Weight:      1.5,
				Cost:        1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: "",
				Wrapper:     "",
			},
			wantErr:          nil,
			expectedDeadline: validTime,
			expectedPackage:  nil,
			expectedWrapper:  nil,
		},
		{
			name: "Negative weight",
			req: orderRequest{
				Weight:      -1.5,
				Cost:        1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
			},
			wantErr: ErrNegativeWeight,
		},
		{
			name: "Zero weight",
			req: orderRequest{
				Weight:      0,
				Cost:        1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
			},
			wantErr: ErrNegativeWeight,
		},
		{
			name: "Negative cost",
			req: orderRequest{
				Weight:      1.5,
				Cost:        -1000,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
			},
			wantErr: ErrNegativeCost,
		},
		{
			name: "Zero cost",
			req: orderRequest{
				Weight:      1.5,
				Cost:        0,
				DeadlineAt:  validTime.Format(timeLayout),
				PackageType: string(packageBox),
			},
			wantErr: ErrNegativeCost,
		},
		{
			name: "Incorrect deadline",
			req: orderRequest{
				Weight:      1.5,
				Cost:        1000,
				DeadlineAt:  "invalid",
				PackageType: string(packageBox),
			},
			wantErr: ErrWrongDeadline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deadline, packageType, wrapperType, err := validateOrderRequest(tt.req)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDeadline.Format(timeLayout), deadline.Format(timeLayout))

				if tt.expectedPackage == nil {
					assert.Nil(t, packageType)
				} else {
					assert.Equal(t, *tt.expectedPackage, *packageType)
				}

				if tt.expectedWrapper == nil {
					assert.Nil(t, wrapperType)
				} else {
					assert.Equal(t, *tt.expectedWrapper, *wrapperType)
				}
			}
		})
	}
}

func TestValidateProcessRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		id      int64
		wantErr error
	}{
		{
			name:    "Handout action",
			action:  "handout",
			id:      1,
			wantErr: nil,
		},
		{
			name:    "Return action",
			action:  "return",
			id:      1,
			wantErr: nil,
		},
		{
			name:    "Invalid action",
			action:  "invalid",
			id:      1,
			wantErr: ErrInvalidAction,
		},
		{
			name:    "Empty action",
			action:  "",
			id:      1,
			wantErr: ErrInvalidAction,
		},
		{
			name:    "Negative ID",
			action:  "handout",
			id:      -1,
			wantErr: ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateProcessRequest(tt.action, tt.id)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseOrderIDFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		orderID string
		want    int64
		wantErr bool
	}{
		{
			name:    "Correct order ID",
			orderID: "123",
			want:    123,
			wantErr: false,
		},
		{
			name:    "Empty order ID",
			orderID: "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "Negative order ID",
			orderID: "-1",
			want:    0,
			wantErr: true,
		},
		{
			name:    "Invalid order ID",
			orderID: "invalid",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseOrderIDFromString(tt.orderID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestValidateCreateUserRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     сreateUserRequest
		wantErr error
	}{
		{
			name: "Valid request",
			req: сreateUserRequest{
				Username: "test",
				Password: "password",
				Role:     "user",
			},
			wantErr: nil,
		},
		{
			name: "Empty username",
			req: сreateUserRequest{
				Username: "",
				Password: "password",
				Role:     "user",
			},
			wantErr: ErrEmptyUsername,
		},
		{
			name: "Empty password",
			req: сreateUserRequest{
				Username: "test",
				Password: "",
				Role:     "user",
			},
			wantErr: ErrEmptyPassword,
		},
		{
			name: "Empty request",
			req: сreateUserRequest{
				Username: "",
				Password: "",
				Role:     "",
			},
			wantErr: ErrEmptyUsername,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateCreateUserRequest(tt.req)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseUserIDFromParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		idParam string
		want    int64
		wantErr error
	}{
		{
			name:    "Correct user ID",
			idParam: "123",
			want:    123,
			wantErr: nil,
		},
		{
			name:    "Empty user ID",
			idParam: "",
			want:    0,
			wantErr: ErrInvalidUserID,
		},
		{
			name:    "Negative user ID",
			idParam: "-1",
			want:    0,
			wantErr: ErrUserIDMustBePositive,
		},
		{
			name:    "Invalid user ID",
			idParam: "invalid",
			want:    0,
			wantErr: ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseUserIDFromParams(tt.idParam)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestValidatePasswordRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "Valid password",
			password: "password",
			wantErr:  nil,
		},
		{
			name:     "Empty password",
			password: "",
			wantErr:  ErrEmptyPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePasswordRequest(tt.password)

			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
