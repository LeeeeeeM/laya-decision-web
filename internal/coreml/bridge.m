#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#include "bridge.h"
#include <stdlib.h>
#include <string.h>

struct laya_ml_model {
	MLModel *model;
};

static char *laya_dup_nsstring(NSString *s) {
	if (!s) {
		return NULL;
	}
	const char *utf8 = [s UTF8String];
	if (!utf8) {
		return NULL;
	}
	return strdup(utf8);
}

static MLComputeUnits laya_units(int compute_units) {
	switch (compute_units) {
	case 1:
		return MLComputeUnitsCPUOnly;
	case 2:
		return MLComputeUnitsCPUAndGPU;
	case 3:
		return MLComputeUnitsCPUAndNeuralEngine;
	default:
		return MLComputeUnitsAll;
	}
}

laya_ml_model *laya_ml_load(const char *path, int compute_units, char **err_out) {
	@autoreleasepool {
		if (err_out) {
			*err_out = NULL;
		}
		if (!path) {
			if (err_out) {
				*err_out = strdup("model path is nil");
			}
			return NULL;
		}
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path] isDirectory:YES];
		NSError *error = nil;

		// Prefer a sibling compiled cache to avoid recompiling on every process start.
		NSString *packagePath = [NSString stringWithUTF8String:path];
		NSString *compiledCache = [[packagePath stringByDeletingLastPathComponent]
			stringByAppendingPathComponent:@"model.mlmodelc"];
		NSURL *compiledURL = nil;
		BOOL isDir = NO;
		if ([[NSFileManager defaultManager] fileExistsAtPath:compiledCache isDirectory:&isDir] && isDir) {
			compiledURL = [NSURL fileURLWithPath:compiledCache isDirectory:YES];
		} else {
			NSURL *tmpCompiled = [MLModel compileModelAtURL:url error:&error];
			if (!tmpCompiled) {
				if (err_out) {
					*err_out = laya_dup_nsstring(error.localizedDescription ?: @"failed to compile MLModel");
				}
				return NULL;
			}
			NSError *copyErr = nil;
			[[NSFileManager defaultManager] removeItemAtPath:compiledCache error:nil];
			if ([[NSFileManager defaultManager] copyItemAtURL:tmpCompiled
			                                           toURL:[NSURL fileURLWithPath:compiledCache isDirectory:YES]
			                                           error:&copyErr]) {
				compiledURL = [NSURL fileURLWithPath:compiledCache isDirectory:YES];
			} else {
				compiledURL = tmpCompiled;
			}
		}
		MLModelConfiguration *cfg = [[MLModelConfiguration alloc] init];
		cfg.computeUnits = laya_units(compute_units);
		error = nil;
		MLModel *model = [MLModel modelWithContentsOfURL:compiledURL configuration:cfg error:&error];
		if (!model) {
			if (err_out) {
				*err_out = laya_dup_nsstring(error.localizedDescription ?: @"failed to load MLModel");
			}
			return NULL;
		}
		laya_ml_model *out = (laya_ml_model *)calloc(1, sizeof(laya_ml_model));
		out->model = model;
		return out;
	}
}

void laya_ml_free(laya_ml_model *model) {
	if (!model) {
		return;
	}
	@autoreleasepool {
		model->model = nil;
	}
	free(model);
}

void laya_ml_free_string(char *s) {
	free(s);
}

void laya_ml_free_outputs(laya_ml_output *outs, int n) {
	if (!outs) {
		return;
	}
	for (int i = 0; i < n; i++) {
		free(outs[i].name);
		free(outs[i].shape);
		free(outs[i].data);
	}
	free(outs);
}

static MLMultiArray *laya_make_f16_array(const laya_ml_input *in, NSError **error) {
	NSMutableArray<NSNumber *> *shape = [NSMutableArray arrayWithCapacity:(NSUInteger)in->ndim];
	for (int i = 0; i < in->ndim; i++) {
		[shape addObject:@(in->shape[i])];
	}
	MLMultiArray *arr = [[MLMultiArray alloc] initWithShape:shape
	                                               dataType:MLMultiArrayDataTypeFloat16
	                                                  error:error];
	if (!arr) {
		return nil;
	}
	memcpy(arr.dataPointer, in->data, in->nbytes);
	return arr;
}

@interface LayaFeatureProvider : NSObject <MLFeatureProvider>
@property(nonatomic, strong) NSDictionary<NSString *, MLFeatureValue *> *dict;
@end

@implementation LayaFeatureProvider
- (instancetype)initWithDictionary:(NSDictionary<NSString *, MLFeatureValue *> *)dict {
	self = [super init];
	if (self) {
		_dict = dict;
	}
	return self;
}
- (NSSet<NSString *> *)featureNames {
	return [NSSet setWithArray:self.dict.allKeys];
}
- (MLFeatureValue *)featureValueForName:(NSString *)featureName {
	return self.dict[featureName];
}
@end

int laya_ml_predict(
	laya_ml_model *model,
	const laya_ml_input *inputs,
	int n_inputs,
	laya_ml_output **outputs_out,
	int *n_outputs_out,
	char **err_out
) {
	@autoreleasepool {
		if (err_out) {
			*err_out = NULL;
		}
		if (!model || !model->model || !inputs || n_inputs <= 0 || !outputs_out || !n_outputs_out) {
			if (err_out) {
				*err_out = strdup("invalid predict arguments");
			}
			return -1;
		}
		NSMutableDictionary<NSString *, MLFeatureValue *> *dict =
			[NSMutableDictionary dictionaryWithCapacity:(NSUInteger)n_inputs];
		for (int i = 0; i < n_inputs; i++) {
			NSError *error = nil;
			MLMultiArray *arr = laya_make_f16_array(&inputs[i], &error);
			if (!arr) {
				if (err_out) {
					*err_out = laya_dup_nsstring(error.localizedDescription ?: @"bad input tensor");
				}
				return -1;
			}
			dict[[NSString stringWithUTF8String:inputs[i].name]] = [MLFeatureValue featureValueWithMultiArray:arr];
		}
		LayaFeatureProvider *provider = [[LayaFeatureProvider alloc] initWithDictionary:dict];
		NSError *error = nil;
		id<MLFeatureProvider> result = [model->model predictionFromFeatures:provider error:&error];
		if (!result) {
			if (err_out) {
				*err_out = laya_dup_nsstring(error.localizedDescription ?: @"prediction failed");
			}
			return -1;
		}
		NSArray<NSString *> *names = result.featureNames.allObjects;
		int n = (int)names.count;
		laya_ml_output *outs = (laya_ml_output *)calloc((size_t)n, sizeof(laya_ml_output));
		for (int i = 0; i < n; i++) {
			NSString *name = names[i];
			MLFeatureValue *value = [result featureValueForName:name];
			MLMultiArray *arr = value.multiArrayValue;
			outs[i].name = laya_dup_nsstring(name);
			outs[i].ndim = (int)arr.shape.count;
			outs[i].shape = (int64_t *)calloc((size_t)outs[i].ndim, sizeof(int64_t));
			size_t nelem = 1;
			for (int d = 0; d < outs[i].ndim; d++) {
				outs[i].shape[d] = arr.shape[d].longLongValue;
				nelem *= (size_t)outs[i].shape[d];
			}
			outs[i].nelem = nelem;
			outs[i].data = (float *)malloc(nelem * sizeof(float));
			if (arr.dataType == MLMultiArrayDataTypeFloat32) {
				memcpy(outs[i].data, arr.dataPointer, nelem * sizeof(float));
			} else if (arr.dataType == MLMultiArrayDataTypeFloat16) {
				const uint16_t *src = (const uint16_t *)arr.dataPointer;
				for (size_t j = 0; j < nelem; j++) {
					uint16_t h = src[j];
					uint32_t sign = ((uint32_t)(h >> 15)) << 31;
					uint32_t exp = (h >> 10) & 0x1f;
					uint32_t mant = h & 0x3ff;
					uint32_t f;
					if (exp == 0) {
						if (mant == 0) {
							f = sign;
						} else {
							exp = 127 - 15 + 1;
							while ((mant & 0x400) == 0) {
								mant <<= 1;
								exp--;
							}
							mant &= 0x3ff;
							f = sign | (exp << 23) | (mant << 13);
						}
					} else if (exp == 31) {
						f = sign | 0x7f800000u | (mant << 13);
					} else {
						f = sign | ((exp + (127 - 15)) << 23) | (mant << 13);
					}
					memcpy(&outs[i].data[j], &f, sizeof(float));
				}
			} else if (arr.dataType == MLMultiArrayDataTypeDouble) {
				const double *src = (const double *)arr.dataPointer;
				for (size_t j = 0; j < nelem; j++) {
					outs[i].data[j] = (float)src[j];
				}
			} else {
				for (size_t j = 0; j < nelem; j++) {
					outs[i].data[j] = arr[j].floatValue;
				}
			}
		}
		*outputs_out = outs;
		*n_outputs_out = n;
		return 0;
	}
}
