package viewsauth

// LoginValues is bound from the login form via `form` tags.
type LoginValues struct {
	Email    string `form:"email"`
	Password string `form:"password"`
	Redirect string `form:"redirect"`
}
