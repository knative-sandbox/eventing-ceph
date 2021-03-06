/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/eventing-ceph/pkg/reconciler/resources"
	v1 "knative.dev/eventing/pkg/apis/sources/v1"
	eventingclient "knative.dev/eventing/pkg/client/clientset/versioned"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

// newSinkBindingCreated makes a new reconciler event with event type Normal, and
// reason SinkBindingCreated.
func newSinkBindingCreated(namespace, name string) pkgreconciler.Event {
	return pkgreconciler.NewEvent(corev1.EventTypeNormal, "SinkBindingCreated", "created SinkBinding: \"%s/%s\"", namespace, name)
}

// newSinkBindingFailed makes a new reconciler event with event type Warning, and
// reason SinkBindingFailed.
func newSinkBindingFailed(namespace, name string, err error) pkgreconciler.Event {
	return pkgreconciler.NewEvent(corev1.EventTypeWarning, "SinkBindingFailed", "failed to create SinkBinding: \"%s/%s\", %w", namespace, name, err)
}

// newSinkBindingUpdated makes a new reconciler event with event type Normal, and
// reason SinkBindingUpdated.
func newSinkBindingUpdated(namespace, name string) pkgreconciler.Event {
	return pkgreconciler.NewEvent(corev1.EventTypeNormal, "SinkBindingUpdated", "updated SinkBinding: \"%s/%s\"", namespace, name)
}

type SinkBindingReconciler struct {
	EventingClientSet eventingclient.Interface
}

func (r *SinkBindingReconciler) ReconcileSinkBinding(ctx context.Context, owner kmeta.OwnerRefable, source duckv1.SourceSpec, subject tracker.Reference) (*v1.SinkBinding, pkgreconciler.Event) {
	expected := resources.MakeSinkBinding(owner, source, subject)

	namespace := owner.GetObjectMeta().GetNamespace()
	sb, err := r.EventingClientSet.SourcesV1().SinkBindings(namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		sb, err = r.EventingClientSet.SourcesV1().SinkBindings(namespace).Create(ctx, expected, metav1.CreateOptions{})
		if err != nil {
			return nil, newSinkBindingFailed(expected.Namespace, expected.Name, err)
		}
		return sb, newSinkBindingCreated(sb.Namespace, sb.Name)
	} else if err != nil {
		return nil, fmt.Errorf("error getting SinkBinding %q: %v", expected.Name, err)
	} else if !metav1.IsControlledBy(sb, owner.GetObjectMeta()) {
		return nil, fmt.Errorf("SinkBinding %q is not owned by %s %q",
			sb.Name, owner.GetGroupVersionKind().Kind, owner.GetObjectMeta().GetName())
	} else if r.specChanged(sb.Spec, expected.Spec) {
		sb.Spec = expected.Spec
		if sb, err = r.EventingClientSet.SourcesV1().SinkBindings(namespace).Update(ctx, sb, metav1.UpdateOptions{}); err != nil {
			return sb, err
		}
		return sb, newSinkBindingUpdated(sb.Namespace, sb.Name)
	} else {
		logging.FromContext(ctx).Debugw("Reusing existing sink binding", zap.Any("sinkBinding", sb))
	}
	return sb, nil
}

func (r *SinkBindingReconciler) specChanged(oldSpec v1.SinkBindingSpec, newSpec v1.SinkBindingSpec) bool {
	return !equality.Semantic.DeepDerivative(newSpec, oldSpec)
}
