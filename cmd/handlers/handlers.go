package handlers

import (
	"net/url"
	"os"
	"reflect"
	ctx "woody-wood-portail/cmd/ctx/auth"

	"github.com/a-h/templ"
	"github.com/go-playground/locales/en"
	"github.com/go-playground/locales/fr"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	fr_translations "github.com/go-playground/validator/v10/translations/fr"
	"github.com/labstack/echo/v4"
)

var (
	GATE_SECRET = os.Getenv("GATE_SECRET")
	BASE_URL    = os.Getenv("BASE_URL")
)

var (
	translator ut.Translator
	validate   = validator.New()

	customValidations = map[string]CustomValidation{}
)

func Render(c echo.Context, statusCode int, t templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
	c.Response().Writer.WriteHeader(statusCode)
	return t.Render(ctx.EchoToTemplContext(c), c.Response().Writer)
}

func Redirect(c echo.Context, url string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", url)
		return c.NoContent(204)
	}
	return c.Redirect(302, url)
}

func RedirectWitQuery(c echo.Context, url string) error {
	query := c.QueryString()
	if query != "" {
		url += "?" + query
	}
	return Redirect(c, url)
}

func Bind[T any](c echo.Context) (*T, url.Values, error) {
	v := new(T)
	if err := c.Bind(v); err != nil {
		return nil, nil, err
	}

	rawValues, _ := c.FormParams()

	return v, rawValues, nil
}

func Validate(c echo.Context, v interface{}) validator.ValidationErrorsTranslations {
	err := validate.StructCtx(ctx.EchoToTemplContext(c), v)
	if err == nil {
		return nil
	}
	validationErr, ok := err.(validator.ValidationErrors)
	if !ok {
		panic(err)
	}

	res := make(validator.ValidationErrorsTranslations, len(validationErr))
	for _, fieldErr := range validationErr {
		res[fieldErr.StructField()] = fieldErr.Translate(translator)
	}

	return res
}

func RegisterValidation(tag string, message string, fn validator.Func, callValidationEvenIfNull ...bool) {
	validate.RegisterValidation(tag, fn, callValidationEvenIfNull...)
	RegisterValidationTranslation(tag, message)
}

func RegisterValidationWithCtx(tag string, message string, fn validator.FuncCtx, callValidationEvenIfNull ...bool) {
	validate.RegisterValidationCtx(tag, fn, callValidationEvenIfNull...)
	RegisterValidationTranslation(tag, message)
}

func RegisterValidationTranslation(tag string, message string) {
	validate.RegisterTranslation(tag, translator, func(ut ut.Translator) error {
		return ut.Add(tag, message, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T(fe.Tag(), fe.Field(), fe.Value().(string))
		return t
	})
}

type Model struct {
	Gates chan struct{}
}

func NewModel() Model {
	return Model{
		Gates: make(chan struct{}, 10),
	}
}

func (model Model) gateConnected() {
	model.Gates <- struct{}{}
}

func (model Model) gateDisconnected() {
	<-model.Gates
}

type CustomValidation struct {
	Validate       validator.Func
	ValidateCtx    validator.FuncCtx
	Message        string
	CallEvenIfNull bool
}

func init() {
	if GATE_SECRET == "" {
		GATE_SECRET = "dev_gate_secret"
	}

	if BASE_URL == "" {
		BASE_URL = "http://localhost"
	}

	universalTranslator := ut.New(en.New(), fr.New())
	translator, _ = universalTranslator.GetTranslator("fr")
	fr_translations.RegisterDefaultTranslations(validate, translator)
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		return field.Tag.Get("tr")
	})

	for tagName, v := range customValidations {
		if v.Validate != nil {
			RegisterValidation(tagName, v.Message, v.Validate, v.CallEvenIfNull)
		} else if v.ValidateCtx != nil {
			RegisterValidationWithCtx(tagName, v.Message, v.ValidateCtx, v.CallEvenIfNull)
		} else {
			panic("Invalid custom validation, Validate or ValidateCtx must be set")
		}
	}
}
