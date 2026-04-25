//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type affiliateTestRepo struct {
	summary  *AffiliateSummary
	invitees []AffiliateInvitee
}

func (r *affiliateTestRepo) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if r.summary == nil {
		return nil, ErrAffiliateProfileNotFound
	}
	return r.summary, nil
}

func (r *affiliateTestRepo) GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error) {
	return nil, ErrAffiliateProfileNotFound
}

func (r *affiliateTestRepo) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	return false, nil
}

func (r *affiliateTestRepo) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64) (bool, error) {
	return false, nil
}

func (r *affiliateTestRepo) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	return 0, 0, nil
}

func (r *affiliateTestRepo) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error) {
	return r.invitees, nil
}

func (r *affiliateTestRepo) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	return nil
}

func (r *affiliateTestRepo) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

func (r *affiliateTestRepo) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	return nil
}

func (r *affiliateTestRepo) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	return nil
}

func (r *affiliateTestRepo) ListUsersWithCustomSettings(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	return nil, 0, nil
}

type affiliateTestSettingRepo map[string]string

func (r affiliateTestSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	value, ok := r[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r affiliateTestSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := r[key]
	if !ok {
		return "", errors.New("setting not found")
	}
	return value, nil
}

func (r affiliateTestSettingRepo) Set(ctx context.Context, key, value string) error {
	return nil
}

func (r affiliateTestSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r affiliateTestSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}

func (r affiliateTestSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r))
	for key, value := range r {
		out[key] = value
	}
	return out, nil
}

func (r affiliateTestSettingRepo) Delete(ctx context.Context, key string) error {
	return nil
}

// TestResolveRebateRatePercent_PerUserOverride verifies that per-inviter
// AffRebateRatePercent overrides the global rate, that NULL falls back to the
// global rate, and that out-of-range exclusive rates are clamped silently.
//
// SettingService is left nil here so globalRebateRatePercent returns the
// documented default (AffiliateRebateRateDefault = 20%) — this exercises the
// fallback path without spinning up a settings stub.
func TestResolveRebateRatePercent_PerUserOverride(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}

	// nil exclusive rate → falls back to global default (20%)
	require.InDelta(t, AffiliateRebateRateDefault,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{}), 1e-9)

	// exclusive rate set → overrides global
	rate := 50.0
	require.InDelta(t, 50.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &rate}), 1e-9)

	// exclusive rate 0 → returns 0 (no rebate, intentional)
	zero := 0.0
	require.InDelta(t, 0.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &zero}), 1e-9)

	// exclusive rate above max → clamped to Max
	tooHigh := 250.0
	require.InDelta(t, AffiliateRebateRateMax,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooHigh}), 1e-9)

	// exclusive rate below min → clamped to Min
	tooLow := -5.0
	require.InDelta(t, AffiliateRebateRateMin,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooLow}), 1e-9)
}

// TestIsEnabled_NilSettingServiceReturnsDefault verifies that IsEnabled
// safely handles a nil settingService dependency by returning the default
// (off). This protects callers from nil-pointer crashes in misconfigured
// environments.
func TestIsEnabled_NilSettingServiceReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}
	require.False(t, svc.IsEnabled(context.Background()))
	require.Equal(t, AffiliateEnabledDefault, svc.IsEnabled(context.Background()))
}

func TestGetAffiliateDetail_IncludesEffectiveAndGlobalRate(t *testing.T) {
	t.Parallel()

	customRate := 33.3
	now := time.Now().UTC()
	svc := &AffiliateService{
		repo: &affiliateTestRepo{
			summary: &AffiliateSummary{
				UserID:               1,
				AffCode:              "VIP2026",
				AffRebateRatePercent: &customRate,
				AffCount:             2,
				AffQuota:             12.5,
				AffHistoryQuota:      45.6,
				CreatedAt:            now,
				UpdatedAt:            now,
			},
			invitees: []AffiliateInvitee{{
				UserID:    2,
				Email:     "alice@example.com",
				Username:  "alice",
				CreatedAt: &now,
			}},
		},
		settingService: &SettingService{
			settingRepo: affiliateTestSettingRepo{
				SettingKeyAffiliateRebateRate: "18.8",
			},
		},
	}

	detail, err := svc.GetAffiliateDetail(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.NotNil(t, detail.AffRebateRatePercent)
	require.InDelta(t, 33.3, *detail.AffRebateRatePercent, 1e-9)
	require.InDelta(t, 33.3, detail.EffectiveRebateRatePercent, 1e-9)
	require.InDelta(t, 18.8, detail.GlobalRebateRatePercent, 1e-9)
	require.Len(t, detail.Invitees, 1)
	require.Equal(t, "a***@e***.com", detail.Invitees[0].Email)
}

func TestGetAffiliateDetail_FallsBackToGlobalRate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	svc := &AffiliateService{
		repo: &affiliateTestRepo{
			summary: &AffiliateSummary{
				UserID:          1,
				AffCode:         "DEFAULT01",
				AffCount:        1,
				AffQuota:        3,
				AffHistoryQuota: 6,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		settingService: &SettingService{
			settingRepo: affiliateTestSettingRepo{
				SettingKeyAffiliateRebateRate: "27.5",
			},
		},
	}

	detail, err := svc.GetAffiliateDetail(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Nil(t, detail.AffRebateRatePercent)
	require.InDelta(t, 27.5, detail.EffectiveRebateRatePercent, 1e-9)
	require.InDelta(t, 27.5, detail.GlobalRebateRatePercent, 1e-9)
}

// TestValidateExclusiveRate_BoundaryAndInvalid covers the validator used by
// admin-facing rate setters: nil is always valid (clear), in-range values
// are accepted, NaN/Inf and out-of-range values produce a typed BadRequest.
func TestValidateExclusiveRate_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateExclusiveRate(nil))

	for _, v := range []float64{0, 0.01, 50, 99.99, 100} {
		v := v
		require.NoError(t, validateExclusiveRate(&v), "value %v should be valid", v)
	}

	for _, v := range []float64{-0.01, 100.01, -100, 200} {
		v := v
		require.Error(t, validateExclusiveRate(&v), "value %v should be rejected", v)
	}

	nan := math.NaN()
	require.Error(t, validateExclusiveRate(&nan))
	posInf := math.Inf(1)
	require.Error(t, validateExclusiveRate(&posInf))
	negInf := math.Inf(-1)
	require.Error(t, validateExclusiveRate(&negInf))
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
}

func TestIsValidAffiliateCodeFormat(t *testing.T) {
	t.Parallel()

	// 邀请码格式校验同时服务于：
	// 1) 系统自动生成的 12 位随机码（A-Z 去 I/O，2-9 去 0/1）
	// 2) 管理员设置的自定义专属码（如 "VIP2026"、"NEW_USER-1"）
	// 因此校验放宽到 [A-Z0-9_-]{4,32}（要求调用方先 ToUpper）。
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical 12-char", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"valid admin custom short", "VIP1", true},
		{"valid admin custom with hyphen", "NEW-USER", true},
		{"valid admin custom with underscore", "VIP_2026", true},
		{"valid 32-char max", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true},
		// Previously-excluded chars (I/O/0/1) are now allowed since admins may use them.
		{"letter I now allowed", "IBCDEFGHJKLM", true},
		{"letter O now allowed", "OBCDEFGHJKLM", true},
		{"digit 0 now allowed", "0BCDEFGHJKLM", true},
		{"digit 1 now allowed", "1BCDEFGHJKLM", true},
		{"too short (3 chars)", "ABC", false},
		{"too long (33 chars)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // bytes out of charset
		{"ascii punctuation .", "ABCDEFGHJK.M", false},
		{"whitespace", "ABCDEFGHJK M", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isValidAffiliateCodeFormat(tc.in))
		})
	}
}
