package srv

// Release retention for the direct channel: manual deletes and
// auto-prune after upload. internal/public releases are never touched —
// they mirror what Play published, and Play owns their lifecycle.

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"srv.exe.dev/db/dbgen"
)

// pruneConfig returns the effective keep-count for a platform: the
// platform's override when set, else the hub-wide Settings value. 0 means
// keep everything (never prune); unset config also means keep everything —
// retention is opt-in.
func (s *Server) pruneConfig(ctx context.Context, plat dbgen.AppPlatform) int {
	if plat.PruneKeep != nil {
		return int(*plat.PruneKeep)
	}
	v, err := dbgen.New(s.DB).GetConfig(ctx, "prune_keep")
	if err != nil || strings.TrimSpace(v) == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// deleteRelease removes a release row and its stored artifact. Returns
// false when the release doesn't exist (caller 404s).
func (s *Server) deleteRelease(ctx context.Context, plat dbgen.AppPlatform, slug string, versionCode int64) (bool, error) {
	q := dbgen.New(s.DB)
	rel, err := q.ReleaseByAppAndCode(ctx, dbgen.ReleaseByAppAndCodeParams{
		AppPlatformID: plat.ID, VersionCode: versionCode,
	})
	if err != nil {
		return false, nil // no rows → not found
	}
	if err := q.DeleteRelease(ctx, dbgen.DeleteReleaseParams{
		AppPlatformID: plat.ID, VersionCode: versionCode,
	}); err != nil {
		return true, err
	}
	_ = s.storage.Delete(ctx, slug+"/"+plat.Platform+"/"+rel.FileName)
	slog.Info("release deleted", "app", slug, "platform", plat.Platform, "versionCode", versionCode)
	return true, nil
}

// pruneDirectReleases keeps only the newest keep direct-channel releases
// for a platform (keep <= 0 never prunes). Called after a successful
// direct upload. Returns the number pruned.
func (s *Server) pruneDirectReleases(ctx context.Context, plat dbgen.AppPlatform, slug string, keep int) int {
	if keep <= 0 {
		return 0
	}
	q := dbgen.New(s.DB)
	rels, err := q.ListReleasesByChannel(ctx, dbgen.ListReleasesByChannelParams{
		AppPlatformID: plat.ID, Channel: "direct",
	})
	if err != nil {
		slog.Warn("prune: list releases", "error", err)
		return 0
	}
	pruned := 0
	for _, rel := range rels[keep:] { // oldest first (list is DESC)
		if _, err := s.deleteRelease(ctx, plat, slug, rel.VersionCode); err != nil {
			slog.Warn("prune: delete release", "versionCode", rel.VersionCode, "error", err)
			continue
		}
		pruned++
	}
	if pruned > 0 {
		slog.Info("direct releases pruned", "app", slug, "platform", plat.Platform, "kept", keep, "pruned", pruned)
	}
	return pruned
}

// parseKeep validates a retention form value: "" = unset/inherit,
// "0" = keep everything, >0 = keep newest N.
func parseKeep(v string) (*int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 || n > 10000 {
		return nil, fmt.Errorf("keep must be a number ≥ 0 (0 = keep everything)")
	}
	return &n, nil
}
