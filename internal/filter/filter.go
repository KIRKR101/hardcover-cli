package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
)

type Expr interface{}

type Comparison struct {
	Field string
	Op    string
	Value any
}

type Logical struct {
	Op    string
	Left  Expr
	Right Expr
}

type Not struct {
	Expr Expr
}

type Field struct {
	Name    string
	Type    string
	Extract func(api.UserBook) any
}

var Fields = map[string]Field{
	"status": {
		Name: "status",
		Type: "enum",
		Extract: func(ub api.UserBook) any {
			return statusIDToName(ub.StatusID)
		},
	},
	"owned": {
		Name: "owned",
		Type: "boolean",
		Extract: func(ub api.UserBook) any {
			return ub.Owned
		},
	},
	"rating": {
		Name: "rating",
		Type: "number",
		Extract: func(ub api.UserBook) any {
			return ub.Rating
		},
	},
	"year": {
		Name: "year",
		Type: "number",
		Extract: func(ub api.UserBook) any {
			if len(ub.DateAdded) >= 4 {
				y, err := strconv.Atoi(ub.DateAdded[:4])
				if err == nil {
					return y
				}
			}
			return 0
		},
	},
	"pages": {
		Name: "pages",
		Type: "number",
		Extract: func(ub api.UserBook) any {
			if ub.Edition != nil && ub.Edition.Pages > 0 {
				return ub.Edition.Pages
			}
			return ub.Book.Pages
		},
	},
	"added": {
		Name: "added",
		Type: "date",
		Extract: func(ub api.UserBook) any {
			return ub.DateAdded
		},
	},
	"title": {
		Name: "title",
		Type: "string",
		Extract: func(ub api.UserBook) any {
			return ub.Book.Title
		},
	},
	"author": {
		Name: "author",
		Type: "string",
		Extract: func(ub api.UserBook) any {
			var authors []string
			for _, c := range ub.Book.Contributions {
				authors = append(authors, c.Author.Name)
			}
			return strings.Join(authors, ", ")
		},
	},
}

func statusIDToName(id int) string {
	names := map[int]string{
		1: "want",
		2: "reading",
		3: "read",
		4: "paused",
		5: "dnf",
		6: "ignored",
	}
	return names[id]
}

func Parse(input string) (Expr, error) {
	lexer := NewLexer(input)
	tokens := lexer.Tokenize()
	parser := NewParser(tokens)
	return parser.Parse()
}

func Eval(expr Expr, ub api.UserBook) (bool, error) {
	switch e := expr.(type) {
	case *Comparison:
		return evalComparison(e, ub)
	case *Logical:
		return evalLogical(e, ub)
	case *Not:
		val, err := Eval(e.Expr, ub)
		if err != nil {
			return false, err
		}
		return !val, nil
	default:
		return false, fmt.Errorf("unknown expression type: %T", expr)
	}
}

func evalComparison(c *Comparison, ub api.UserBook) (bool, error) {
	field, ok := Fields[c.Field]
	if !ok {
		return false, fmt.Errorf("unknown field %q: %w", c.Field, errs.ErrInvalid)
	}

	fieldVal := field.Extract(ub)

	switch field.Type {
	case "string":
		return evalString(fieldVal, c.Op, c.Value)
	case "number":
		return evalNumber(fieldVal, c.Op, c.Value)
	case "boolean":
		return evalBoolean(fieldVal, c.Op, c.Value)
	case "date":
		return evalDate(fieldVal, c.Op, c.Value)
	case "enum":
		return evalEnum(fieldVal, c.Op, c.Value)
	default:
		return false, fmt.Errorf("unsupported field type: %s", field.Type)
	}
}

func evalString(fieldVal any, op string, value any) (bool, error) {
	s, ok := fieldVal.(string)
	if !ok {
		return false, fmt.Errorf("field is not a string")
	}

	v, ok := value.(string)
	if !ok {
		return false, fmt.Errorf("value is not a string")
	}

	switch op {
	case "=":
		return strings.Contains(strings.ToLower(s), strings.ToLower(v)), nil
	case "!=":
		return !strings.Contains(strings.ToLower(s), strings.ToLower(v)), nil
	case "~":
		re, err := regexp.Compile("(?i)" + v)
		if err != nil {
			return false, fmt.Errorf("invalid regex: %w", err)
		}
		return re.MatchString(s), nil
	case "!~":
		re, err := regexp.Compile("(?i)" + v)
		if err != nil {
			return false, fmt.Errorf("invalid regex: %w", err)
		}
		return !re.MatchString(s), nil
	default:
		return false, fmt.Errorf("unsupported operator for string: %s", op)
	}
}

func evalNumber(fieldVal any, op string, value any) (bool, error) {
	f, ok := fieldVal.(float64)
	if !ok {
		if i, ok := fieldVal.(int); ok {
			f = float64(i)
		} else {
			return false, fmt.Errorf("field is not a number")
		}
	}

	v, ok := value.(float64)
	if !ok {
		return false, fmt.Errorf("value is not a number")
	}

	switch op {
	case "=":
		return f == v, nil
	case "!=":
		return f != v, nil
	case ">":
		return f > v, nil
	case "<":
		return f < v, nil
	case ">=":
		return f >= v, nil
	case "<=":
		return f <= v, nil
	default:
		return false, fmt.Errorf("unsupported operator for number: %s", op)
	}
}

func evalBoolean(fieldVal any, op string, value any) (bool, error) {
	b, ok := fieldVal.(bool)
	if !ok {
		return false, fmt.Errorf("field is not a boolean")
	}

	v, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("value is not a boolean")
	}

	switch op {
	case "=":
		return b == v, nil
	case "!=":
		return b != v, nil
	default:
		return false, fmt.Errorf("unsupported operator for boolean: %s", op)
	}
}

func evalDate(fieldVal any, op string, value any) (bool, error) {
	s, ok := fieldVal.(string)
	if !ok {
		return false, fmt.Errorf("field is not a date string")
	}

	v, ok := value.(string)
	if !ok {
		return false, fmt.Errorf("value is not a date string")
	}

	fieldDate, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return false, fmt.Errorf("invalid field date: %w", err)
	}

	valueDate, err := time.Parse("2006-01-02", v)
	if err != nil {
		return false, fmt.Errorf("invalid value date: %w", err)
	}

	switch op {
	case "=":
		return fieldDate.Equal(valueDate), nil
	case "!=":
		return !fieldDate.Equal(valueDate), nil
	case ">":
		return fieldDate.After(valueDate), nil
	case "<":
		return fieldDate.Before(valueDate), nil
	case ">=":
		return fieldDate.After(valueDate) || fieldDate.Equal(valueDate), nil
	case "<=":
		return fieldDate.Before(valueDate) || fieldDate.Equal(valueDate), nil
	default:
		return false, fmt.Errorf("unsupported operator for date: %s", op)
	}
}

func evalEnum(fieldVal any, op string, value any) (bool, error) {
	s, ok := fieldVal.(string)
	if !ok {
		return false, fmt.Errorf("field is not a string")
	}

	v, ok := value.(string)
	if !ok {
		return false, fmt.Errorf("value is not a string")
	}

	switch op {
	case "=":
		return strings.EqualFold(s, v), nil
	case "!=":
		return !strings.EqualFold(s, v), nil
	default:
		return false, fmt.Errorf("unsupported operator for enum: %s", op)
	}
}

func evalLogical(l *Logical, ub api.UserBook) (bool, error) {
	left, err := Eval(l.Left, ub)
	if err != nil {
		return false, err
	}

	switch l.Op {
	case "AND":
		if !left {
			return false, nil
		}
		return Eval(l.Right, ub)
	case "OR":
		if left {
			return true, nil
		}
		return Eval(l.Right, ub)
	default:
		return false, fmt.Errorf("unknown logical operator: %s", l.Op)
	}
}
