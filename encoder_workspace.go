package silk

// Each encoder owns its temporary frame buffers. No state or scratch is shared
// between recordings, or between the normal and FEC quantizers.
type encoderFrameWorkspace struct {
	control              encoder_control_FIX
	xfw, pInHP, resPitch *slice[int16]
	lbrrPayload          *slice[byte]
}

func (e *encoder_state_FIX) frameScratch() *encoderFrameWorkspace {
	if e.frameWorkspace == nil {
		w := &encoderFrameWorkspace{
			xfw:         alloc[int16](MAX_FRAME_LENGTH),
			pInHP:       alloc[int16](MAX_FRAME_LENGTH),
			resPitch:    alloc[int16](2*MAX_FRAME_LENGTH + LA_PITCH_MAX),
			lbrrPayload: alloc[byte](MAX_ARITHM_BYTES),
		}
		w.control.init()
		e.frameWorkspace = w
	}
	w := e.frameWorkspace
	// Fresh scratch was zero-initialized every frame before reuse was added.
	// Preserve that behavior for every buffer, including unused frame tails.
	memset(w.xfw, int16(0), len(w.xfw.arr))
	memset(w.pInHP, int16(0), len(w.pInHP.arr))
	memset(w.resPitch, int16(0), len(w.resPitch.arr))
	memset(w.lbrrPayload, byte(0), len(w.lbrrPayload.arr))
	w.control.reset()
	return w
}

func (e *encoder_control_FIX) reset() {
	// Retain only the allocated buffers; reset all scalar and inline-array
	// fields through a struct assignment so future scalar fields also reset.
	*e = encoder_control_FIX{
		sCmn: encoder_control{
			GainsIndices: e.sCmn.GainsIndices, NLSFIndices: e.sCmn.NLSFIndices,
			LTPIndex: e.sCmn.LTPIndex, pitchL: e.sCmn.pitchL,
		},
		Gains_Q16: e.Gains_Q16, PredCoef_Q12: e.PredCoef_Q12, LTPCoef_Q14: e.LTPCoef_Q14,
		AR1_Q13: e.AR1_Q13, AR2_Q13: e.AR2_Q13, LF_shp_Q14: e.LF_shp_Q14,
		Tilt_Q14: e.Tilt_Q14, HarmShapeGain_Q14: e.HarmShapeGain_Q14,
		input_quality_bands_Q15: e.input_quality_bands_Q15, ResNrg: e.ResNrg, ResNrgQ: e.ResNrgQ,
	}
	for _, buffer := range []*slice[int32]{e.sCmn.GainsIndices, e.sCmn.NLSFIndices, e.sCmn.LTPIndex, e.sCmn.pitchL,
		e.Gains_Q16, e.LF_shp_Q14, e.Tilt_Q14, e.HarmShapeGain_Q14, e.input_quality_bands_Q15, e.ResNrg, e.ResNrgQ} {
		memset(buffer, int32(0), len(buffer.arr))
	}
	for _, buffer := range []*slice[int16]{e.PredCoef_Q12[0], e.PredCoef_Q12[1], e.LTPCoef_Q14, e.AR1_Q13, e.AR2_Q13} {
		memset(buffer, int16(0), len(buffer.arr))
	}
}
