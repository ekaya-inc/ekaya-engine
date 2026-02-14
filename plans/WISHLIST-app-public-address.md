# WISHLIST: Public Address App

**Status:** Wishlist (out of scope for v1)

## Idea

An Ekaya application ("Public Address") that gives ekaya-engine a public HTTPS URL without any TLS/cert configuration.

ekaya-engine opens a secure tunnel to a service running in us.[dev.]ekaya.ai. The Ekaya service authenticates and proxies calls from the public address back to the user's ekaya-engine instance.

This eliminates the entire HTTPS configuration problem for users who don't need to self-host their own domain/certs.
