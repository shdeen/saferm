#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <string.h>

// saferm_register_notifier registers the login item that has the given bundle
// identifier, unless it is already enabled or awaiting the user's approval. It
// returns NULL on success, or an error description that the caller must free.
char *saferm_register_notifier(const char *identifier) {
    @autoreleasepool {
        SMAppService *service = [SMAppService loginItemServiceWithIdentifier:[NSString stringWithUTF8String:identifier]];
        if (service.status == SMAppServiceStatusEnabled || service.status == SMAppServiceStatusRequiresApproval) {
            return NULL;
        }
        NSError *error = nil;
        if (![service registerAndReturnError:&error]) {
            return strdup(error.localizedDescription.UTF8String);
        }
        return NULL;
    }
}
