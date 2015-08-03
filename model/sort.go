package model

// SortableTask type to sort tasks by dependency count.
type SortableTask struct {
	Tasks []Task
}

func (st *SortableTask) Len() int {
	return len(st.Tasks)
}

func (st *SortableTask) Less(i, j int) bool {
	if len(st.Tasks[i].DependsOn) > len(st.Tasks[j].DependsOn) {
		return false
	}
	if len(st.Tasks[i].DependsOn) < len(st.Tasks[j].DependsOn) {
		return true
	}
	return st.Tasks[i].DisplayName < st.Tasks[j].DisplayName
}

func (st *SortableTask) Swap(i, j int) {
	st.Tasks[i], st.Tasks[j] = st.Tasks[j], st.Tasks[i]
}
