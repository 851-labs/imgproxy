package processing

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/imagetype"
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/options/keys"
)

func TestMainPipelineRunsStickerTraceBeforeGeometrySteps(t *testing.T) {
	pipeline := new(Processor).mainPipeline()

	colorspaceStepIndex := pipelineStepIndex(pipeline, "colorspaceToProcessing")
	stickerTraceStepIndex := pipelineStepIndex(pipeline, "stickerTrace")
	cropStepIndex := pipelineStepIndex(pipeline, "crop")
	scaleStepIndex := pipelineStepIndex(pipeline, "scale")

	if colorspaceStepIndex < 0 || stickerTraceStepIndex < 0 || cropStepIndex < 0 || scaleStepIndex < 0 {
		t.Fatalf(
			"expected pipeline steps were not found: colorspace=%d sticker_trace=%d crop=%d scale=%d",
			colorspaceStepIndex,
			stickerTraceStepIndex,
			cropStepIndex,
			scaleStepIndex,
		)
	}

	if stickerTraceStepIndex <= colorspaceStepIndex {
		t.Fatalf(
			"expected sticker_trace after colorspace conversion, got colorspace=%d sticker_trace=%d",
			colorspaceStepIndex,
			stickerTraceStepIndex,
		)
	}

	if stickerTraceStepIndex >= cropStepIndex {
		t.Fatalf(
			"expected sticker_trace before crop, got sticker_trace=%d crop=%d",
			stickerTraceStepIndex,
			cropStepIndex,
		)
	}

	if stickerTraceStepIndex >= scaleStepIndex {
		t.Fatalf(
			"expected sticker_trace before scale, got sticker_trace=%d scale=%d",
			stickerTraceStepIndex,
			scaleStepIndex,
		)
	}
}

func TestMainPipelineRunsDropShadowAfterFixSizeBeforeFlatten(t *testing.T) {
	pipeline := new(Processor).mainPipeline()

	fixSizeStepIndex := pipelineStepIndex(pipeline, "fixSize")
	dropShadowStepIndex := pipelineStepIndex(pipeline, "dropShadow")
	flattenStepIndex := pipelineStepIndex(pipeline, "flatten")

	if fixSizeStepIndex < 0 || dropShadowStepIndex < 0 || flattenStepIndex < 0 {
		t.Fatalf(
			"expected pipeline steps were not found: fix_size=%d drop_shadow=%d flatten=%d",
			fixSizeStepIndex,
			dropShadowStepIndex,
			flattenStepIndex,
		)
	}

	if dropShadowStepIndex <= fixSizeStepIndex {
		t.Fatalf(
			"expected drop_shadow after fix_size, got fix_size=%d drop_shadow=%d",
			fixSizeStepIndex,
			dropShadowStepIndex,
		)
	}

	if dropShadowStepIndex >= flattenStepIndex {
		t.Fatalf(
			"expected drop_shadow before flatten, got drop_shadow=%d flatten=%d",
			dropShadowStepIndex,
			flattenStepIndex,
		)
	}
}

func TestCanScaleOnLoadDisabledForStickerTrace(t *testing.T) {
	config := NewDefaultConfig()
	processor := &Processor{config: &config}
	sourceImageData := imagedata.NewFromBytesWithFormat(imagetype.JPEG, []byte{1, 2, 3})
	defer sourceImageData.Close()

	stickerTraceOptions := options.New()
	stickerTraceOptions.Set(keys.StickerTrace, true)

	contextWithStickerTrace := &Context{
		PO: ProcessingOptions{
			Options: stickerTraceOptions,
			config:  &config,
		},
		ImgData: sourceImageData,
	}

	if processor.canScaleOnLoad(contextWithStickerTrace, 2) {
		t.Fatal("expected scale-on-load to be disabled when sticker_trace is enabled")
	}

	defaultOptions := options.New()
	contextWithoutStickerTrace := &Context{
		PO: ProcessingOptions{
			Options: defaultOptions,
			config:  &config,
		},
		ImgData: sourceImageData,
	}

	if !processor.canScaleOnLoad(contextWithoutStickerTrace, 2) {
		t.Fatal("expected scale-on-load to stay enabled for JPEG when sticker_trace is disabled")
	}
}

func pipelineStepIndex(pipeline Pipeline, methodName string) int {
	methodSuffix := ".(*Processor)." + methodName + "-fm"

	for index, step := range pipeline {
		stepName := runtime.FuncForPC(reflect.ValueOf(step).Pointer()).Name()
		if strings.HasSuffix(stepName, methodSuffix) {
			return index
		}
	}

	return -1
}
