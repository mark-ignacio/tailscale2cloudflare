# tailscale2cloudflare

A tiny utility to run periodically that updates DNS entries for your Tailscale devices in a Cloudflare zone. It's currently being used to keep a globally-distributed K3S cluster running.

It can(not):

- [x] Create A records based on Tailscale hostnames
- [x] Update existing A records
- [x] Support subdomain suffixes
- [ ] Support multiple A records for a host

`grep -F TODO` to see the various complicated things that need to be done.
