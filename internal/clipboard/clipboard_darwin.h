#ifndef REMEMBER_FIX_CLIPBOARD_DARWIN_H
#define REMEMBER_FIX_CLIPBOARD_DARWIN_H

#include <stddef.h>
#include <stdint.h>

uint32_t rf_pasteboard_change_count(void);
char *rf_pasteboard_text(void);
char *rf_frontmost_app(void);
void *rf_pasteboard_png(size_t *length);

#endif
