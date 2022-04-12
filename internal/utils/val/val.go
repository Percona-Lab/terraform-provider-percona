package val

func Str(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}
