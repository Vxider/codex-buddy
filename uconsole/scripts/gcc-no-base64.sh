#!/usr/bin/env bash
set -euo pipefail

real_cc="${REAL_CC:-}"
if [[ -z "${real_cc}" ]]; then
  if command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then
    real_cc="$(command -v aarch64-linux-gnu-gcc)"
  else
    real_cc="$(command -v gcc)"
  fi
fi

needs_patch=0
for arg in "$@"; do
  if [[ "${arg}" == *"c_glfw_lin.cgo2.c" ]]; then
    needs_patch=1
    break
  fi
done

if [[ "${needs_patch}" -eq 0 ]]; then
  exec "${real_cc}" "$@"
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/gcc-no-base64.XXXXXX")"
trap 'rm -rf -- "${tmpdir}"' EXIT

mkdir -p "${tmpdir}/glfw/src"

repo_root=""
args=()
for ((i=1; i<=$#; i++)); do
  arg="${!i}"
  args+=("${arg}")
  if [[ "${arg}" == "-I" ]]; then
    next_index=$((i + 1))
    if [[ ${next_index} -le $# ]]; then
      next_arg="${!next_index}"
      if [[ "${next_arg}" == *"/github.com/go-gl/glfw/"* ]]; then
        repo_root="${next_arg}"
      fi
    fi
  fi
done

if [[ -z "${repo_root}" ]]; then
  exec "${real_cc}" "$@"
fi

ln -s "${repo_root}/glfw/src/internal.h" "${tmpdir}/glfw/src/internal.h"

patched_file="${tmpdir}/glfw/src/linux_joystick.c"
cp "${repo_root}/glfw/src/linux_joystick.c" "${patched_file}"

perl -0pi -e 's@        static const char stateMap\[3\]\[3\] =\n        \{\n            \{ GLFW_HAT_CENTERED, GLFW_HAT_UP,       GLFW_HAT_DOWN \},\n            \{ GLFW_HAT_LEFT,     GLFW_HAT_LEFT_UP,  GLFW_HAT_LEFT_DOWN \},\n            \{ GLFW_HAT_RIGHT,    GLFW_HAT_RIGHT_UP, GLFW_HAT_RIGHT_DOWN \},\n        \};\n\n        const int hat = \(code - ABS_HAT0X\) / 2;\n        const int axis = \(code - ABS_HAT0X\) % 2;\n        int\* state = js->linjs.hats\[hat\];\n\n        // NOTE: Looking at several input drivers, it seems all hat events use\n        //       -1 for left / up, 0 for centered and 1 for right / down\n        if \(value == 0\)\n            state\[axis\] = 0;\n        else if \(value < 0\)\n            state\[axis\] = 1;\n        else if \(value > 0\)\n            state\[axis\] = 2;\n\n        _glfwInputJoystickHat\(js, index, stateMap\[state\[0\]\]\[state\[1\]\]\);@        const int hat = (code - ABS_HAT0X) / 2;\n        const int axis = (code - ABS_HAT0X) % 2;\n        int* state = js->linjs.hats[hat];\n        unsigned char hatState = GLFW_HAT_CENTERED;\n\n        // NOTE: Looking at several input drivers, it seems all hat events use\n        //       -1 for left / up, 0 for centered and 1 for right / down\n        if (value == 0)\n            state[axis] = 0;\n        else if (value < 0)\n            state[axis] = 1;\n        else if (value > 0)\n            state[axis] = 2;\n\n        if (state[0] == 0)\n        {\n            if (state[1] == 1)\n                hatState = GLFW_HAT_UP;\n            else if (state[1] == 2)\n                hatState = GLFW_HAT_DOWN;\n        }\n        else if (state[0] == 1)\n        {\n            if (state[1] == 0)\n                hatState = GLFW_HAT_LEFT;\n            else if (state[1] == 1)\n                hatState = GLFW_HAT_LEFT_UP;\n            else\n                hatState = GLFW_HAT_LEFT_DOWN;\n        }\n        else\n        {\n            if (state[1] == 0)\n                hatState = GLFW_HAT_RIGHT;\n            else if (state[1] == 1)\n                hatState = GLFW_HAT_RIGHT_UP;\n            else\n                hatState = GLFW_HAT_RIGHT_DOWN;\n        }\n\n        _glfwInputJoystickHat(js, index, hatState);@s' "${patched_file}"

patched_args=("-I" "${tmpdir}")
patched_args+=("${args[@]}")

exec "${real_cc}" "${patched_args[@]}"
