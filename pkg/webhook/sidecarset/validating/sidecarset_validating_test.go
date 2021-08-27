package validating

import (
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateSidecarSet(t *testing.T) {
	errorCases := map[string]appsv1alpha1.SidecarSet{
		"missing-selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
						ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
							Type: appsv1alpha1.ShareVolumePolicyDisabled,
						},
						UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
							UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
						},
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"wrong-updateStrategy": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
					ScatterStrategy: appsv1alpha1.UpdateScatterStrategy{
						{
							Key:   "key-1",
							Value: "value-1",
						},
						{
							Key:   "key-1",
							Value: "value-1",
						},
					},
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
						ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
							Type: appsv1alpha1.ShareVolumePolicyDisabled,
						},
						UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
							UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
						},
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"wrong-selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app-name",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"app-group", "app-risk-"},
						},
					},
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
						ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
							Type: appsv1alpha1.ShareVolumePolicyEnabled,
						},
						UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
							UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
						},
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"wrong-initContainer": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
				},
				InitContainers: []appsv1alpha1.SidecarContainer{
					{
						Container: corev1.Container{
							Name:                     "test-sidecar",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"missing-container": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
				},
			},
		},
		"wrong-containers": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,

						UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
							UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
						},
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"wrong-volumes": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
					Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
						ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
							Type: appsv1alpha1.ShareVolumePolicyDisabled,
						},
						UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
							UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
						},
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "test-volume",
					},
				},
			},
		},
	}

	for name, sidecarSet := range errorCases {
		allErrs := validateSidecarSetSpec(&sidecarSet, field.NewPath("spec"))
		if len(allErrs) != 1 {
			t.Errorf("%v: expect errors len 1, but got: %v", name, allErrs)
		} else {
			fmt.Printf("%v: %v\n", name, allErrs)
		}
	}
}

func TestSelectorConflict(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "i"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"b": "i"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "i",
						"c": "j",
					},
				},
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "x",
						"c": "j",
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"h", "i", "j"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"i", "j"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"h", "i"},
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
	}

	for i, testCase := range testCases {
		output := util.IsSelectorOverlapping(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestSidecarSetVolumeConflict(t *testing.T) {
	sidecarsetList := &appsv1alpha1.SidecarSetList{
		Items: []appsv1alpha1.SidecarSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
				},
			},
		},
	}
	sidecarset := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"a": "b"},
			},
		},
	}

	cases := []struct {
		name              string
		getSidecarSet     func() *appsv1alpha1.SidecarSet
		getSidecarSetList func() *appsv1alpha1.SidecarSetList
		expectErrLen      int
	}{
		{
			name: "sidecarset volume name different",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-3",
					},
					{
						Name: "volume-4",
					},
				}
				return newSidecarList
			},
			expectErrLen: 0,
		},
		{
			name: "sidecarset volume name same and equal",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work",
							},
						},
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work",
							},
						},
						Name: "volume-2",
					},
					{
						Name: "volume-3",
					},
				}
				return newSidecarList
			},
			expectErrLen: 0,
		},
		{
			name: "sidecarset volume name same, but not equal",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work-1",
							},
						},
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work-2",
							},
						},
					},
					{
						Name: "volume-3",
					},
				}
				return newSidecarList
			},
			expectErrLen: 1,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			sidecarset := cs.getSidecarSet()
			sidecarsetList := cs.getSidecarSetList()
			errs := validateSidecarConflict(sidecarsetList, sidecarset, field.NewPath("spec"), &SidecarSetConfig{})
			if len(errs) != cs.expectErrLen {
				t.Fatalf("except ErrLen(%d), but get errs(%d)", cs.expectErrLen, len(errs))
			}
		})
	}
}

func TestSidecarSetNameConflict(t *testing.T) {
	sidecarset := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{Name: "same-name"},
				},
			},
		},
	}
	cases := []struct {
		name             string
		getSidecarSet    func() *appsv1alpha1.SidecarSet
		getOthers        func() *appsv1alpha1.SidecarSetList
		sidecarSetConfig *SidecarSetConfig
		exceptErrs       int
	}{
		{
			name: "two selector don't contains same key, and loose overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"c": "d",
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: true,
			},
			exceptErrs: 0,
		},
		{
			name: "two selector don't contains same key, and strict overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"c": "d",
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: false,
			},
			exceptErrs: 1,
		},
		{
			name: "two selector have same key, and loose overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "c",
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: false,
			},
			exceptErrs: 0,
		},
		{
			name: "two selector have same key, and strict overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "c",
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: false,
			},
			exceptErrs: 0,
		},
		{
			name: "two selector don't contains same MatchExpressions key, and strict overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app2",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: false,
			},
			exceptErrs: 1,
		},
		{
			name: "two selector don't contains same MatchExpressions key, and loose overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app2",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: true,
			},
			exceptErrs: 0,
		},
		{
			name: "two selector contains same MatchExpressions key, and loose overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: true,
			},
			exceptErrs: 0,
		},
		{
			name: "two selector contains same MatchExpressions key, and strict overlap",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				object := sidecarset.DeepCopy()
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return object
			},
			getOthers: func() *appsv1alpha1.SidecarSetList {
				object := sidecarset.DeepCopy()
				object.Name = "sidecarset1"
				object.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"a": "b",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app1",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				}
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{*object},
				}
			},
			sidecarSetConfig: &SidecarSetConfig{
				IsSelectorLooseOverlap: false,
			},
			exceptErrs: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			allErrs := validateSidecarConflict(cs.getOthers(), cs.getSidecarSet(), field.NewPath("spec.containers"), cs.sidecarSetConfig)
			if len(allErrs) != cs.exceptErrs {
				t.Errorf("expect errors len %d, but got: %v", cs.exceptErrs, len(allErrs))
			}
		})
	}
}

type TestCase struct {
	Input  [2]metav1.LabelSelector
	Output bool
}

func TestSelectorExclusion(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{ //0
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"e", "f"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //1
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"e", "f", "c"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //2
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //3
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c", "e", "f"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //4
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"c", "e", "f"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //5
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //6
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //7
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //8
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //9
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //10
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{ //11
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "b",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //12
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "b",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{ //13
				{
					MatchLabels: map[string]string{
						"a": "b",
					},
				},
				{
					MatchLabels: map[string]string{
						"c": "d",
					},
				},
			},
			Output: false,
		},
	}

	for i, testCase := range testCases {
		output := isSelectorLooseOverlap(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestSelectorLooseOverlap(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "e", "f"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c", "f"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: true,
		},
	}

	for i, testCase := range testCases {
		output := util.IsLabelSelectorOverlapped(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestIsMatchExpOverlap(t *testing.T) {
	cases := []struct {
		name      string
		matchExp1 func() metav1.LabelSelectorRequirement
		matchExp2 func() metav1.LabelSelectorRequirement
		except    bool
	}{
		{
			name: "LabelSelectorOpIn and LabelSelectorOpExists, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and SelectorOpExists, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "d"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c", "e"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "e"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpExists and LabelSelectorOpDoesNotExist, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"c"},
				}
			},
			except: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			overlap := isMatchExpOverlap(cs.matchExp1(), cs.matchExp2())
			if cs.except != overlap {
				t.Fatalf("isMatchExpOverlap(%s) except(%v) but get(%v)", cs.name, cs.except, overlap)
			}
		})
	}
}

func TestIsMatchExpExclusion(t *testing.T) {
	cases := []struct {
		name      string
		matchExp1 func() metav1.LabelSelectorRequirement
		matchExp2 func() metav1.LabelSelectorRequirement
		except    bool
	}{
		{
			name: "LabelSelectorOpIn and LabelSelectorOpIn, and exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"e", "f", "e"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpIn, and not exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpDoesNotExist, and exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpExists, and not exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c", "e"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and not exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "e"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpExists and LabelSelectorOpDoesNotExist, and exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"c"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and not exclusion",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			overlap := isMatchExpExclusion(cs.matchExp1(), cs.matchExp2())
			if cs.except != overlap {
				t.Fatalf("isMatchExpOverlap(%s) except(%v) but get(%v)", cs.name, cs.except, overlap)
			}
		})
	}
}
