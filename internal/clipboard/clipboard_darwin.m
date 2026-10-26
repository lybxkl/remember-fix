#import "clipboard_darwin.h"
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

uint32_t rf_pasteboard_change_count(void) {
	@autoreleasepool {
		return (uint32_t)[[NSPasteboard generalPasteboard] changeCount];
	}
}

char *rf_pasteboard_text(void) {
	@autoreleasepool {
		NSString *text = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
		if (text == nil) {
			return NULL;
		}
		const char *utf8 = [text UTF8String];
		return utf8 == NULL ? NULL : strdup(utf8);
	}
}

char *rf_frontmost_app(void) {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		NSString *name = [app localizedName];
		if (name == nil) {
			return NULL;
		}
		const char *utf8 = [name UTF8String];
		return utf8 == NULL ? NULL : strdup(utf8);
	}
}

// 优先取 PNG；截图工具常只写 TIFF，用 AppKit 转成 PNG 再交给 Go。
void *rf_pasteboard_png(size_t *length) {
	@autoreleasepool {
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		NSData *data = [pasteboard dataForType:NSPasteboardTypePNG];
		if (data == nil) {
			NSData *tiff = [pasteboard dataForType:NSPasteboardTypeTIFF];
			if (tiff == nil) {
				return NULL;
			}
			NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
			if (rep == nil) {
				return NULL;
			}
			data = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			if (data == nil) {
				return NULL;
			}
		}
		NSUInteger n = [data length];
		if (n == 0) {
			return NULL;
		}
		void *buffer = malloc(n);
		if (buffer == NULL) {
			return NULL;
		}
		memcpy(buffer, [data bytes], n);
		*length = (size_t)n;
		return buffer;
	}
}
