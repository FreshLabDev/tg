// SPDX-License-Identifier: Apache-2.0

// Package tg is the Telegram Bot API client shared by the Asterfield bots.
//
// It exists because the same transport was living in three repositories at
// once and had already drifted: one copy counted metrics, one redacted errors
// differently, and only one knew about a self-hosted Bot API server — that one
// incorrectly. Everything the Telegram protocol dictates belongs here;
// everything the product dictates (what counts as speech, how a panel reads,
// which language a user gets) stays in the bot.
//
// The module has no dependencies outside the standard library. Logging is a
// *slog.Logger and metrics are an [Observer] callback, so no bot inherits
// another bot's choice of instrumentation.
//
// Two failure modes motivated the design and are handled explicitly:
//
//   - A Bot API server older than the bot answers "method not found" to every
//     new method, and the bot goes silent instead of failing. [Client.Preflight]
//     turns that into a refusal to start. See [Client.Probe].
//   - A server started with --local answers getFile with an absolute path on
//     its own disk and serves nothing over HTTP. [Client.DownloadToFile] reads
//     from disk in that case and never falls back to a download that cannot
//     work. See [WithLocalFiles].
package tg

// BotAPI is the Bot API version this module is written against. It is what
// CI compares with Telegram's published version to notice that the module has
// fallen behind, and what a bot can report in its own build info.
const BotAPI = "10.4"

// Upload limits, in bytes. Telegram's own server takes an upload of up to 50
// MB. A server started with --local takes a file of up to 2000 MB, which is
// Telegram's limit for an account without Premium: 4000 parts of 512 KB.
// Both megabytes are binary.
//
// A --local server lets a larger file through and Telegram refuses it only
// once the whole of it has been uploaded, so a bot checks the size before it
// sends. A photo, a thumbnail and a live photo have tighter limits of their
// own.
const (
	MaxCloudUploadBytes = 50 << 20
	MaxLocalUploadBytes = 2000 << 20
)
