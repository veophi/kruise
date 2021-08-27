package validating

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// 判断 Selector 是否 overlap（含义：selector1、selector2 包含相同的key，且 两者之间存在交集）
// 1. 当selector1、selector2 中不包含相同的key时，认为 非 overlap，例如：a=b 和 c=d
// 2. 当selector1、selector2 中包含相同的key时，且 matchLabels & matchExps 重叠时， 认为 overlap
//    a In [b,c] 和 a Exist
//                  a In [b,...] [c,...] [包含b,c任意即可,...]
//                  a NotIn [a,...] [b,....] [c,....] [同时包含b,c除外，其它情况都可以...] [b,c,e]
//    a Exist    和 a Exist
//                  a In [a,b,...] [任意即可，...]
//                  a NotIn [a,b,...] [任意即可，...]
//    a NotIn [b,c] 和 a Exist
//                  a NotExist
//                  a NotIn [a,b...] [任意即可,...]
//                  a In [a,b] [a,c] [e,f] 除[b],[c],[b,c]之外其它任何即可
//    a NotExist 和 a NotExist
//                  a NotIn [任意即可,...]，因为它包含 NotExist 语义
//    当selector1、selector2 中包含相同的key时，除上述情况外 其它认为 非overlap
//
func isSelectorLooseOverlap(selector1, selector2 *metav1.LabelSelector) bool {
	matchExp1 := convertSelectorToMatchExpressions(selector1)
	matchExp2 := convertSelectorToMatchExpressions(selector2)

	for k, exp1 := range matchExp1 {
		exp2, ok := matchExp2[k]
		if !ok {
			return false
		}

		if !isMatchExpOverlap(exp1, exp2) {
			return false
		}
	}

	for k, exp2 := range matchExp2 {
		exp1, ok := matchExp1[k]
		if !ok {
			return false
		}

		if !isMatchExpOverlap(exp2, exp1) {
			return false
		}
	}

	return true
}

func isMatchExpOverlap(matchExp1, matchExp2 metav1.LabelSelectorRequirement) bool {
	switch matchExp1.Operator {
	case metav1.LabelSelectorOpIn:
		if matchExp2.Operator == metav1.LabelSelectorOpExists {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpIn && sliceOverlaps(matchExp2.Values, matchExp1.Values) {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpNotIn && !sliceContains(matchExp2.Values, matchExp1.Values) {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpExists:
		if matchExp2.Operator == metav1.LabelSelectorOpIn || matchExp2.Operator == metav1.LabelSelectorOpNotIn ||
			matchExp2.Operator == metav1.LabelSelectorOpExists {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpNotIn:
		if matchExp2.Operator == metav1.LabelSelectorOpExists || matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist ||
			matchExp2.Operator == metav1.LabelSelectorOpNotIn {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpIn && !sliceContains(matchExp1.Values, matchExp2.Values) {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpDoesNotExist:
		if matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist || matchExp2.Operator == metav1.LabelSelectorOpNotIn {
			return true
		} else {
			return false
		}
	}

	return false
}

// 判断 Selector 是否互斥（含义就是：selector1、selector2 绝对不会匹配到同一个 pod）
// -- 只有存在同一个 key 且 MatchLabels & MatchExpression 的时候，才认为互斥
// 1. selector1、selector2 中不包含相同的 key，则认为不 互斥
// 2. selector1、selector2 中包含相同的 key，且 MatchLabels & MatchExpression 互斥
//    a In [b,c] 互斥 a In [e,f,...] [包含非(b,c)的任意值]
//                    a NotExist
//                    a  NotIn [b,c,...]
//    a Exist    互斥 a NotExist
//    a NotIn [b,c] 互斥 a In [b] [c] [b,c]
//    a NotExist 互斥 a Exist
//                   a In [任意值...]
// 除 以上情况外，其它场景 都认为 不互斥
func isSelectorExclusion(selector1, selector2 *metav1.LabelSelector) bool {
	matchExp1 := convertSelectorToMatchExpressions(selector1)
	matchExp2 := convertSelectorToMatchExpressions(selector2)

	for k, exp1 := range matchExp1 {
		if exp2, ok := matchExp2[k]; ok && isMatchExpExclusion(exp1, exp2) {
			return true
		}
	}

	for k, exp2 := range matchExp2 {
		if exp1, ok := matchExp1[k]; ok && isMatchExpExclusion(exp2, exp1) {
			return true
		}
	}

	return false
}

func isMatchExpExclusion(matchExp1, matchExp2 metav1.LabelSelectorRequirement) bool {
	switch matchExp1.Operator {
	case metav1.LabelSelectorOpIn:
		if matchExp2.Operator == metav1.LabelSelectorOpIn && !sliceOverlaps(matchExp2.Values, matchExp1.Values) {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpNotIn && sliceContains(matchExp2.Values, matchExp1.Values) {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpExists:
		if matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpNotIn:
		if matchExp2.Operator == metav1.LabelSelectorOpIn && sliceContains(matchExp1.Values, matchExp2.Values) {
			return true
		} else {
			return false
		}
	case metav1.LabelSelectorOpDoesNotExist:
		if matchExp2.Operator == metav1.LabelSelectorOpExists || matchExp2.Operator == metav1.LabelSelectorOpIn {
			return true
		} else {
			return false
		}
	}

	return false
}

// 注意：本方法不考虑selector中配置的 MatchLabels、MatchExpressions 包含相同 key 的场景 -- 太复杂，并且实际线上不太可能这样配置
func convertSelectorToMatchExpressions(selector *metav1.LabelSelector) map[string]metav1.LabelSelectorRequirement {
	matchExps := map[string]metav1.LabelSelectorRequirement{}
	for _, exp := range selector.MatchExpressions {
		matchExps[exp.Key] = exp
	}

	for k, v := range selector.MatchLabels {
		matchExps[k] = metav1.LabelSelectorRequirement{
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{v},
		}
	}

	return matchExps
}

func sliceOverlaps(a, b []string) bool {
	keyExist := make(map[string]bool, len(a))
	for _, key := range a {
		keyExist[key] = true
	}
	for _, key := range b {
		if keyExist[key] {
			return true
		}
	}
	return false
}

// a contains b
func sliceContains(a, b []string) bool {
	keyExist := make(map[string]bool, len(a))
	for _, key := range a {
		keyExist[key] = true
	}
	for _, key := range b {
		if !keyExist[key] {
			return false
		}
	}
	return true
}
