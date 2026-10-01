package dto

// mapSlice luôn trả về slice khác nil để JSON ra [] thay vì null.
func mapSlice[T, R any](in []T, fn func(*T) R) []R {
	out := make([]R, 0, len(in))
	for i := range in {
		out = append(out, fn(&in[i]))
	}
	return out
}
