package pagination

const DefaultLimit = 25

type Params struct {
	Limit  int
	Offset int
}

func Normalize(params Params, defaultLimit, maxLimit int) Params {
	if defaultLimit <= 0 {
		defaultLimit = DefaultLimit
	}
	if maxLimit > 0 && defaultLimit > maxLimit {
		defaultLimit = maxLimit
	}
	if params.Limit <= 0 {
		params.Limit = defaultLimit
	}
	if maxLimit > 0 && params.Limit > maxLimit {
		params.Limit = maxLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	return params
}

func Offset(page, limit int) int {
	if page <= 1 || limit <= 0 {
		return 0
	}
	return (page - 1) * limit
}

func Page(offset, limit int) int {
	if limit <= 0 {
		return 1
	}
	if offset < 0 {
		offset = 0
	}
	return offset/limit + 1
}
