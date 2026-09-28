// DICOM Part 10 sintético: quadrado 32x32, sem qualquer paciente real.
export function syntheticDICOM({ calibrated = true, width = 32, height = 32 } = {}) {
  const element = (group, tag, vr, value) => {
    let data = Buffer.isBuffer(value) ? value : Buffer.from(value, 'ascii');
    if (data.length % 2) data = Buffer.concat([data, Buffer.from([vr === 'UI' ? 0 : 32])]);
    const long = ['OB', 'OW'].includes(vr);
    const header = Buffer.alloc(long ? 12 : 8);
    header.writeUInt16LE(group, 0); header.writeUInt16LE(tag, 2); header.write(vr, 4);
    if (long) header.writeUInt32LE(data.length, 8); else header.writeUInt16LE(data.length, 6);
    return Buffer.concat([header, data]);
  };
  const us = (n) => { const b = Buffer.alloc(2); b.writeUInt16LE(n); return b; };
  const uid = '2.25.123456789';
  const sop = '1.2.840.10008.5.1.4.1.1.7'; // Secondary Capture
  const meta = Buffer.concat([
    element(2, 1, 'OB', Buffer.from([0, 1])), element(2, 2, 'UI', sop),
    element(2, 3, 'UI', uid), element(2, 0x10, 'UI', '1.2.840.10008.1.2.1'),
    element(2, 0x12, 'UI', '2.25.987654321'),
  ]);
  const size = Buffer.alloc(4); size.writeUInt32LE(meta.length);
  const pixels = Buffer.alloc(width * height * 2);
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) pixels.writeUInt16LE((x * 80 + y * 20) % 4096, 2 * (width * y + x));
  return Buffer.concat([
    Buffer.alloc(128), Buffer.from('DICM'), element(2, 0, 'UL', size), meta,
    element(8, 0x16, 'UI', sop), element(8, 0x18, 'UI', uid), element(8, 0x60, 'CS', 'OT'),
    element(0x10, 0x10, 'PN', 'FICTICIO^VIEWER'), element(0x10, 0x20, 'LO', 'SYNTH-VIEWER'),
    element(0x20, 0x0d, 'UI', '2.25.111111111'), element(0x20, 0x0e, 'UI', '2.25.222222222'),
    element(0x20, 0x52, 'UI', '2.25.333333333'),
    element(0x20, 0x11, 'IS', '1'), element(0x20, 0x13, 'IS', '1'),
    element(0x20, 0x32, 'DS', '0\\0\\0'), element(0x20, 0x37, 'DS', '1\\0\\0\\0\\1\\0'),
    element(0x28, 2, 'US', us(1)), element(0x28, 4, 'CS', 'MONOCHROME2'),
    element(0x28, 0x10, 'US', us(height)), element(0x28, 0x11, 'US', us(width)),
    ...(calibrated ? [element(0x28, 0x30, 'DS', '1\\1')] : []), element(0x28, 0x100, 'US', us(16)),
    element(0x28, 0x101, 'US', us(12)), element(0x28, 0x102, 'US', us(11)), element(0x28, 0x103, 'US', us(0)),
    element(0x28, 0x1050, 'DS', '2048'), element(0x28, 0x1051, 'DS', '4096'),
    element(0x7fe0, 0x10, 'OW', pixels),
  ]);
}
