package main

import (
	"testing"

	"cyberteam/internal/profile"
)

// --- getBossName ---

func TestGetBossName_NilProfile(t *testing.T) {
	gBossProfile = nil
	if got := getBossName(); got != "Boss" {
		t.Errorf("getBossName() = %q, want %q", got, "Boss")
	}
}

func TestGetBossName_FromProfile(t *testing.T) {
	gBossProfile = &profile.Profile{Name: "Kai"}
	defer func() { gBossProfile = nil }()
	if got := getBossName(); got != "Kai" {
		t.Errorf("getBossName() = %q, want %q", got, "Kai")
	}
}

func TestGetBossName_EmptyProfileName(t *testing.T) {
	gBossProfile = &profile.Profile{Name: ""}
	defer func() { gBossProfile = nil }()
	if got := getBossName(); got != "Boss" {
		t.Errorf("getBossName() = %q, want %q (empty name should fallback)", got, "Boss")
	}
}

// --- getNameToRole / getOnlineStaffNames (no Manager) ---

func TestGetNameToRole_NilBoss(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	got := getNameToRole()
	if len(got) != 0 {
		t.Errorf("expected empty map when gBoss is nil, got %v", got)
	}
}

func TestGetOnlineStaffNames_NilBoss(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	got := getOnlineStaffNames()
	if len(got) != 0 {
		t.Errorf("expected empty slice when gBoss is nil, got %v", got)
	}
}

// --- roleByStaffID ---

func TestRoleByStaffID(t *testing.T) {
	cases := []struct {
		staffID string
		want    string
	}{
		{"developer-1234567890", "developer"},
		{"product-9876543210", "product"},
		{"tester-111", "tester"},
		{"nohyphen", "nohyphen"}, // 无连字符，返回原值
	}
	for _, c := range cases {
		if got := roleByStaffID(c.staffID); got != c.want {
			t.Errorf("roleByStaffID(%q) = %q, want %q", c.staffID, got, c.want)
		}
	}
}

// --- extractNameFromReply ---

func TestExtractNameFromReply(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"**Sarah**: 我来分析需求", "Sarah"},
		{"[mtg-abc] **Alex**: 好的收到", "Alex"},
		{"no bold format here", ""},
		{"****: 空名字", ""},
	}
	for _, c := range cases {
		if got := extractNameFromReply(c.content); got != c.want {
			t.Errorf("extractNameFromReply(%q) = %q, want %q", c.content, got, c.want)
		}
	}
}

// --- extractMentions (gBoss nil → 退化到原样返回) ---

func TestExtractMentions_NilBoss(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	// 无法映射名字，@Alex 直接作为 role 返回
	mentions := extractMentions("@Alex 评估一下这个需求")
	if len(mentions) != 1 || mentions[0] != "Alex" {
		t.Errorf("extractMentions = %v, want [Alex]", mentions)
	}
}

func TestExtractMentions_NoMentions(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	mentions := extractMentions("大家好，今天开会")
	if len(mentions) != 0 {
		t.Errorf("expected no mentions, got %v", mentions)
	}
}

// --- getSenderColor ---

func TestGetSenderColor_BossRole(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	if got := getSenderColor("boss"); got != ColorPurple {
		t.Errorf("getSenderColor(boss) = %q, want ColorPurple", got)
	}
}

func TestGetSenderColor_Unknown(t *testing.T) {
	orig := gBoss
	gBoss = nil
	defer func() { gBoss = orig }()

	if got := getSenderColor("unknown"); got != ColorWhite {
		t.Errorf("getSenderColor(unknown) = %q, want ColorWhite", got)
	}
}
