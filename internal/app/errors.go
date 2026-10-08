package app

import (
	"fmt"
	"time"
)

// Error and warning codes (spec §10). The UI translates them.
const (
	CodeNotAdmin = "NOT_ADMIN"
	// Linux GUI ↔ daemon (the same strings as in internal/rpc).
	CodeDaemonUnreachable = "DAEMON_UNREACHABLE"
	CodeDaemonProtocol    = "DAEMON_PROTOCOL_MISMATCH"
	CodeNotAuthorized     = "NOT_AUTHORIZED"
	CodeNoUI              = "NO_UI"
	// The GUI's service buttons (Linux): pkexec failed (the cause is the
	// command to run in a terminal); the tar.gz installs with install.sh;
	// the daemon and Windows have no such buttons.
	CodePkexecFailed             = "PKEXEC_FAILED"
	CodeServiceInstallManual     = "SERVICE_INSTALL_MANUAL"
	CodeServiceActionUnsupported = "SERVICE_ACTION_UNSUPPORTED"
	// Log code: the system changed DNS and Ghostline set it again (Linux).
	CodeDNSReapplied         = "DNS_REAPPLIED"
	CodePort53Busy           = "PORT53_BUSY"
	CodePort53NotOwner       = "PORT53_NOT_OWNER"
	CodeNoServers            = "NO_SERVERS"
	CodeEngineSelfTest       = "ENGINE_SELFTEST_FAILED"
	CodeSetDNSFailed         = "SET_DNS_FAILED"
	CodeVerifyLeak           = "VERIFY_LEAK"
	CodeRestoreFailed        = "RESTORE_FAILED"
	CodeDPIStartFailed       = "DPI_START_FAILED"
	CodeDPIBlockedByAV       = "DPI_BLOCKED_BY_AV"
	CodeDPIKernelUnsupported = "DPI_KERNEL_UNSUPPORTED"
	CodeDPIDriverBusy        = "DPI_DRIVER_BUSY"
	// CodeSessionPending: the system proxy waits for a user's desktop
	// session (Linux, before login).
	CodeSessionPending = "SESSION_PENDING"
	// CodeProxyDesktopUnsupported: Ghostline cannot set this desktop's proxy.
	CodeProxyDesktopUnsupported = "PROXY_DESKTOP_UNSUPPORTED"
	// CodeCertPartial: a Fake SNI root is in the system store but not in
	// every browser's (Linux: Firefox's policy or a user's NSS database).
	CodeCertPartial = "CERT_PARTIAL"
	// CodeCertNSSToolMissing: Chrome's per-user store needs certutil
	// (Ubuntu: libnss3-tools).
	CodeCertNSSToolMissing = "CERT_NSS_TOOL_MISSING"
	CodeDPIHashMismatch    = "DPI_HASH_MISMATCH"
	CodeServerListBadSig   = "SERVERLIST_BAD_SIGNATURE"
	CodeUpdateCheckFailed  = "UPDATE_CHECK_FAILED"
	CodeAutotuneNoPreset   = "AUTOTUNE_NO_PRESET"
	CodeInternal           = "INTERNAL"
	CodeSettingsReset      = "SETTINGS_RESET"
	CodeNotConnected       = "NOT_CONNECTED"

	// Phase 2A (spec 2A section 10).
	CodeProxyPortBusy     = "PROXY_PORT_BUSY"
	CodeProxySelfTest     = "PROXY_SELFTEST_FAILED"
	CodeProxyFirewall     = "PROXY_FIREWALL"
	CodeSysProxyExisting  = "SYSPROXY_EXISTING"
	CodeSysProxyFailed    = "SYSPROXY_FAILED"
	CodeSysProxyTakenOver = "SYSPROXY_TAKEN_OVER"
	// CodeTestDomainBroken: a test domain failed on most servers and was
	// ignored (params: domains).
	CodeTestDomainBroken = "TEST_DOMAIN_BROKEN"
	// CodeTestDomainNoAddress: a new test domain has no IPv4 address, so
	// every server would fail on it (the cause names the domain).
	CodeTestDomainNoAddress = "TEST_DOMAIN_NO_ADDRESS"
	CodeSysProxyRestore     = "SYSPROXY_RESTORE_FAILED"
	CodeRulesParse          = "RULES_PARSE"
	CodeListFetch           = "LIST_FETCH_FAILED"
	CodeListUnsupported     = "LIST_UNSUPPORTED_FORMAT"
	CodeListTooLarge        = "LIST_TOO_LARGE"
	CodeUpstreamProxy       = "UPSTREAM_PROXY_FAILED"
	CodeNoPinnedServers     = "NO_PINNED_SERVERS"
	CodeDPIBlacklistEmpty   = "DPI_BLACKLIST_EMPTY"
	// DPI engines (zapret2 spec §9.4).
	CodeDPIFallback            = "DPI_FALLBACK"
	CodeAutotuneEngineSwitched = "AUTOTUNE_ENGINE_SWITCHED"
	CodeStrategyListInvalid    = "STRATEGY_LIST_INVALID"
	CodeDPICustomRejected      = "DPI_CUSTOM_REJECTED"

	// Phase 2B.
	CodeDNSServerPortInUse   = "DNSSERVER_PORT_IN_USE"
	CodeDNSServerFirewall    = "DNSSERVER_FIREWALL"
	CodeDNSServerSelfTest    = "DNSSERVER_SELFTEST_FAILED"
	CodeCertInstallFailed    = "CERT_INSTALL_FAILED"
	CodeCertRemoveFailed     = "CERT_REMOVE_FAILED"
	CodeCertKeyUnreadable    = "CERT_KEY_UNREADABLE"
	CodeListSignatureInvalid = "LIST_SIGNATURE_INVALID"
	CodeSetupPageFailed      = "SETUP_PAGE_FAILED"
	CodeFakeSNINeedsProxy    = "FAKESNI_NEEDS_PROXY"
	CodeFakeSNITooMany       = "FAKESNI_TOO_MANY"
	CodeFakeSNISelfTest      = "FAKESNI_SELFTEST_FAILED"
	CodeFakeSNINotAcked      = "FAKESNI_NOT_ACKED"

	// Phase 3 (spec 3 section 12).
	CodeToolBusy             = "TOOL_BUSY"
	CodeLookupNotConnected   = "LOOKUP_NOT_CONNECTED"
	CodeLookupBadName        = "LOOKUP_BAD_NAME"
	CodeScanTooMany          = "SCAN_TOO_MANY"
	CodeCFScanNoNetwork      = "CFSCAN_NO_NETWORK"
	CodeCFScanHostInvalid    = "CFSCAN_HOST_INVALID"
	CodeStampInvalid         = "STAMP_INVALID"
	CodeImportInvalid        = "IMPORT_INVALID"
	CodeImportWhileConnected = "IMPORT_WHILE_CONNECTED"
	CodeImportExpired        = "IMPORT_EXPIRED"
	CodeImportWriteFailed    = "IMPORT_WRITE_FAILED"
	CodeExportWriteFailed    = "EXPORT_WRITE_FAILED"
)

// AppError is a coded error the UI can translate.
type AppError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
	cause  error
}

// Error is "CODE" or "CODE: cause"; the UI translates the leading code.
func (e *AppError) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code
}

func (e *AppError) Unwrap() error { return e.cause }

func appErr(code string, cause error, kv ...any) *AppError {
	e := &AppError{Code: code, cause: cause}
	if len(kv) > 0 {
		e.Params = map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i].(string)] = kv[i+1]
		}
	}
	return e
}

// NoPinnedError means "use pinned servers only" is on but no pinned server
// is usable (none pinned, or none passed the check).
type NoPinnedError struct {
	Checked int
}

func (e *NoPinnedError) Error() string {
	return fmt.Sprintf("no usable pinned server (%d checked)", e.Checked)
}

// NoServersError is returned by a Picker that found no working server.
type NoServersError struct {
	Checked int
	Elapsed time.Duration
}

func (e *NoServersError) Error() string {
	return fmt.Sprintf("no working DNS servers (%d checked in %s)", e.Checked, e.Elapsed)
}
