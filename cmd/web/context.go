package main

type contextKey string

const isAuthenticatedContextKey = contextKey("isAuthenticated")

// The logged-in User's name and theme, for every page.
const viewerContextKey = contextKey("viewer")
