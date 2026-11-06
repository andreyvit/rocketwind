// Extract Unicode data from the same ICU version used by the reference Node.
// Development tooling only; Rocketwind does not link to ICU or use cgo.
#include <unicode/ucol.h>
#include <unicode/ucoleitr.h>
#include <unicode/unorm2.h>
#include <unicode/uchar.h>
#include <unicode/ustring.h>
#include <unicode/uset.h>
#include <stdio.h>
#include <stdlib.h>

static UCollator *collator;
static UErrorCode error;
static void check(void) {
    if (U_FAILURE(error)) { fprintf(stderr, "%s\n", u_errorName(error)); exit(1); }
}
static int elements(const UChar *text, int length, uint32_t *out) {
    UCollationElements *it = ucol_openElements(collator, text, length, &error);
    check();
    int count = 0;
    int32_t value;
    while ((value = ucol_next(it, &error)) != UCOL_NULLORDER) {
        if (count == 64) { fprintf(stderr,"too many collation elements\n"); exit(1); }
        out[count++] = (uint32_t)value;
    }
    ucol_closeElements(it);
    check();
    return count;
}
static void printElements(uint32_t *values, int count) {
    for (int i = 0; i < count; i++) printf(" %08X", values[i]);
    puts("");
}
static void printCodePoints(const UChar *s, int length) {
    for (int i = 0; i < length;) {
        UChar32 cp;
        U16_NEXT(s, i, length, cp);
        printf(" %X", cp);
    }
}
int main(void) {
    collator = ucol_open("en_US", &error);
    const UNormalizer2 *normalizer = unorm2_getNFDInstance(&error);
    check();
    UVersionInfo unicodeVersion;
    u_getUnicodeVersion(unicodeVersion);
    fprintf(stderr,"ICU %s, Unicode %d.%d.%d\n",U_ICU_VERSION,unicodeVersion[0],unicodeVersion[1],unicodeVersion[2]);
    uint32_t values[64];
    for (UChar32 cp = 0; cp <= 0x10ffff; cp++) {
        if (cp >= 0xd800 && cp <= 0xdfff) continue;
        UChar text[2];
        int32_t length = 0;
        U16_APPEND_UNSAFE(text, length, cp);
        int count = elements(text, length, values);
        // ICU's algorithmic weights for unassigned characters need no table entry.
        uint32_t x = cp + 1;
        uint32_t primary = 2 + (x % 18) * 14; x /= 18;
        primary |= (2 + (x % 254)) << 8; x /= 254;
        primary |= (4 + (x % 251)) << 16;
        primary |= 0xfe000000;
        int implicit = count == 2 && values[0] == ((primary & 0xffff0000) | 0x0505) && values[1] == ((primary << 16) | 0xc0);
        if (!implicit && !(cp >= 0xac00 && cp <= 0xd7a3)) {
            printf("C %X",cp); printElements(values,count);
        }
        uint8_t ccc = u_getCombiningClass(cp);
        if (ccc != 0) printf("K %X %u\n",cp,ccc);
        if (cp >= 0xac00 && cp <= 0xd7a3) continue;
        UChar decomposition[64];
        length = unorm2_getDecomposition(normalizer,cp,decomposition,64,&error);
        check();
        if (length > 0) { printf("D %X",cp); printCodePoints(decomposition,length); puts(""); }
    }
    USet *contractions = uset_openEmpty();
    ucol_getContractionsAndExpansions(collator, contractions, NULL, true, &error);
    check();
    for (int i = uset_getRangeCount(contractions); i < uset_getItemCount(contractions); i++) {
        UChar text[64]; UChar32 start,end;
        int length = uset_getItem(contractions,i,&start,&end,text,64,&error);
        check();
        int count = elements(text,length,values);
        printf("T");printCodePoints(text,length);printf(" :");printElements(values,count);
    }
    uset_close(contractions);
    ucol_close(collator);
}
