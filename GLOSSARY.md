# Smugbox

A self-hosted photo gallery that Lightroom Classic publishes to and visitors browse.

## Photos

**Photo**:
One image in an album, identified by its Lightroom photo UUID within that album.
_Avoid_: Image, picture, asset

**Original**:
The file exactly as Lightroom exported it; every variant is rendered from it.
_Avoid_: Master, source, upload

**Variant**:
A display rendition of an original at a fixed size (thumb, small, medium, large) or the tiny blurred placeholder (blur).
_Avoid_: Derivative, rendition, size

**Pending**:
A photo whose variants do not yet match its original; visitors never see it.
_Avoid_: Processing, hidden, unready

**Ready**:
A photo whose variants were rendered from its current original; visitors see it.
_Avoid_: Visible, published, done

**Replace**:
Publishing a new original for an existing photo, which makes it pending again.
_Avoid_: Re-upload, overwrite, update (for the file)

**Photo library**:
The part of the backend that owns a photo's original, its variants and whether it is pending or ready.
_Avoid_: Photo service, storage
