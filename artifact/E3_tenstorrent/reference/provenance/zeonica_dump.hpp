// SPDX-License-Identifier: Apache-2.0
// Zeonica data-capture hook. No-op unless ZEONICA_TT_DUMP_DIR is set.
// Writes the exact host vectors written to / read from device buffers so a
// separate simulator can consume identical inputs and compare outputs.
#pragma once
#include <cstdint>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <map>
#include <string>
#include <vector>

namespace zeonica_dump {

inline const char* dir() { return std::getenv("ZEONICA_TT_DUMP_DIR"); }

inline std::string unique_base(const std::string& name) {
    static std::map<std::string, int> seen;
    int n = seen[name]++;
    return std::string(dir()) + "/" + (n == 0 ? name : name + "_" + std::to_string(n));
}

// meta is a JSON object body without braces, e.g. "\"format\":\"bf16\",\"layout\":\"tile_nfaces\"".
inline void u32(const std::string& name, const std::vector<uint32_t>& v, const std::string& meta = "") {
    if (!dir()) {
        return;
    }
    std::string base = unique_base(name);
    std::ofstream f(base + ".u32", std::ios::binary);
    f.write(reinterpret_cast<const char*>(v.data()), v.size() * sizeof(uint32_t));
    std::ofstream m(base + ".json");
    m << "{\"name\":\"" << name << "\",\"words\":" << v.size() << (meta.empty() ? "" : ",") << meta << "}";
}

// 16-bit element vectors (e.g. bfloat16) in their host order.
template <typename T>
inline void u16(const std::string& name, const std::vector<T>& v, const std::string& meta = "") {
    static_assert(sizeof(T) == 2, "u16 dump expects 16-bit elements");
    if (!dir()) {
        return;
    }
    std::string base = unique_base(name);
    std::ofstream f(base + ".u16", std::ios::binary);
    f.write(reinterpret_cast<const char*>(v.data()), v.size() * 2);
    std::ofstream m(base + ".json");
    m << "{\"name\":\"" << name << "\",\"elements\":" << v.size() << (meta.empty() ? "" : ",") << meta << "}";
}

}  // namespace zeonica_dump
