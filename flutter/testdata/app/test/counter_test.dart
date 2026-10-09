import 'package:flutter_test/flutter_test.dart';

import 'package:app/counter.dart';

void main() {
  test('increment adds one', () {
    expect(increment(0), 1);
    expect(increment(41), 42);
  });
}
