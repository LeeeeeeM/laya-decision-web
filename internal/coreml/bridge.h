#ifndef LAYA_COREML_BRIDGE_H
#define LAYA_COREML_BRIDGE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct laya_ml_model laya_ml_model;

// compute_units: 0=all, 1=cpu_only, 2=cpu_and_gpu, 3=cpu_and_ne
laya_ml_model *laya_ml_load(const char *path, int compute_units, char **err_out);
void laya_ml_free(laya_ml_model *model);

typedef struct {
	const char *name;
	const int64_t *shape;
	int ndim;
	const void *data; // float16 little-endian bytes
	size_t nbytes;
} laya_ml_input;

typedef struct {
	char *name;
	int64_t *shape;
	int ndim;
	float *data; // float32
	size_t nelem;
} laya_ml_output;

// Caller frees each output with laya_ml_free_output, then the array with free().
int laya_ml_predict(
	laya_ml_model *model,
	const laya_ml_input *inputs,
	int n_inputs,
	laya_ml_output **outputs_out,
	int *n_outputs_out,
	char **err_out
);

void laya_ml_free_outputs(laya_ml_output *outs, int n);
void laya_ml_free_string(char *s);

#ifdef __cplusplus
}
#endif

#endif
