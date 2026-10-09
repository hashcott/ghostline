# Security policy

Ghostline runs with administrator rights on Windows and as a root service on Linux, and it changes system DNS, so we take security reports seriously.

## Reporting a vulnerability

Please **do not open a public issue.** Report privately through GitHub:
**[Security → Report a vulnerability](https://github.com/hashcott/ghostline/security/advisories/new)**.

Include the affected version, the steps to reproduce, and the impact. We aim to reply within 7 days and will credit you in the advisory unless you prefer otherwise.

## Supported versions

Only the latest release receives security fixes.

## In scope

- DNS leaks, or ways to bypass the encrypted path while Ghostline reports "protected".
- Failure to restore the original DNS.
- Privilege escalation through Ghostline, its watchdog or its scheduled tasks (Windows), or through the Linux service: its control socket, the session agent it runs as a user, `pkexec`/`--install-system`, and the package scripts.
- Bypassing the signature checks on the server list, the DNSCrypt list, the zapret2 strategy list, or the bundled zapret2 and GoodbyeDPI binaries.

Vulnerabilities in upstream projects (dnsproxy, zapret2, GoodbyeDPI, WinDivert, WebView2, WebKitGTK) should be reported to those projects. Please let us know as well if Ghostline needs to update.
